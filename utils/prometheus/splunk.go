package prometheus

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"

	"github.com/splunk/splunk-cloud-sdk-go/sdk"
	"github.com/splunk/splunk-cloud-sdk-go/services"
	"github.com/splunk/splunk-cloud-sdk-go/services/search"
)

type SplunkProvider struct {
    client         *sdk.Client
	Project        string
	Stage          string
	Service        string
	DeploymentType string
	Labels         map[string]string
	CustomFilters  []*keptnv2.SLIFilter
	CustomQueries  map[string]string
}

func NewSplunkProvider( Host string, Tenant string, Token string, eventData *keptnv2.EventData, deploymentType string, labels map[string]string, customFilters []*keptnv2.SLIFilter) (*SplunkProvider, error) {
    // Create a new Splunk SDK client
    client, err := sdk.NewClient(&services.Config{
		Host: 		Host,
        Tenant:     Tenant,
        Token:     	Token,
    })

    if err != nil {
        return nil, err
    }

    return &SplunkProvider{
        client: 		client,     
        Project:        eventData.Project,
		Stage:          eventData.Stage,
		Service:        eventData.Service,
		DeploymentType: deploymentType,
		Labels:         labels,
		CustomFilters:  customFilters,  
    }, nil
}

func (sp *SplunkProvider) GetSLI(metric string, startTime string, endTime string) (float64, error) {

    startUnix, err := parseUnixTimestamp(startTime)
	if err != nil {
		return 0, fmt.Errorf("unable to parse start timestamp: %w", err)
	}
	endUnix, err := parseUnixTimestamp(endTime)
	if err != nil {
		return 0, fmt.Errorf("unable to parse end timestamp: %w", err)
	}

    // Create a new search job
    query, err := sp.GetMetricQuery(metric, startUnix, endUnix)
    if err != nil {
		return 0, fmt.Errorf("unable to get metric query: %w", err)
	}

	su := startUnix.Format("2006-01-02T15:04:05.000-0700")
	eu := endUnix.Format("2006-01-02T15:04:05.000-0700")

    jobResp, err := sp.client.SearchService.CreateJob(search.SearchJob{
        Query: query,
        ResolvedEarliest: &su,
        ResolvedLatest: &eu,
    })
    if err != nil {
        return 0, fmt.Errorf("failed to create search job: %w", err)
    }

    // Wait for the search job to complete
    sp.client.SearchService.WaitForJob(*jobResp.Sid, time.Second)

    // Fetch the search results
    searchResp, err :=  sp.client.SearchService.ListResults(*jobResp.Sid, nil)
    if err != nil {
        return 0, fmt.Errorf("failed to get search results: %w", err)
    }

    // Parse the search results and calculate the SLI
    sli := 0.0
    for _, result := range searchResp.Results {

        sliValue, err := strconv.ParseFloat(result[metric].(string), 64)
        if err != nil {
            return 0, fmt.Errorf("failed to parse %v: %w", metric, err)
        }
        sli = sliValue

    }

    return sli, nil
}

func (sp *SplunkProvider) GetMetricQuery(metric string, start time.Time, end time.Time) (string, error) {
	query := sp.CustomQueries[metric]
	if query != "" {
		query = sp.replaceQueryParameters(query, start, end)

		return query, nil
	}
    return "", errors.New("No Custom query specified")  //MK: customized error because the default queries were deleted

}

func (sp *SplunkProvider) replaceQueryParameters(query string, start time.Time, end time.Time) string {
    //MK: This code in the loop might be specific to prometheus, i'll
	for _, filter := range sp.CustomFilters {
		filter.Value = strings.Replace(filter.Value, "'", "", -1)
		filter.Value = strings.Replace(filter.Value, "\"", "", -1)
		query = strings.Replace(query, "$"+filter.Key, filter.Value, -1)
		query = strings.Replace(query, "$"+strings.ToUpper(filter.Key), filter.Value, -1)
	}
	query = strings.Replace(query, "$PROJECT", sp.Project, -1)
	query = strings.Replace(query, "$STAGE", sp.Stage, -1)
	query = strings.Replace(query, "$SERVICE", sp.Service, -1)
	query = strings.Replace(query, "$DEPLOYMENT", sp.DeploymentType, -1)

	// replace labels
	for key, value := range sp.Labels {
		query = strings.Replace(query, "$LABEL."+key, value, -1)
	}

	// replace duration
	durationString := strconv.FormatInt(getDurationInSeconds(start, end), 10) + "s"

	query = strings.Replace(query, "$DURATION_SECONDS", durationString, -1)
	return query
}

func parseUnixTimestamp(timestamp string) (time.Time, error) {
	parsedTime, err := time.Parse(time.RFC3339, timestamp)
	if err == nil {
		return parsedTime, nil
	}

	timestampInt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return time.Now(), err
	}
	unix := time.Unix(timestampInt, 0)
	return unix, nil
}

func getDurationInSeconds(start, end time.Time) int64 {
	seconds := end.Sub(start).Seconds()
	return int64(math.Ceil(seconds))
}