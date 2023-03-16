package eventhandling

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/kelseyhightower/envconfig"
	"github.com/keptn/go-utils/pkg/api/models"
	api "github.com/keptn/go-utils/pkg/api/utils"
	keptncommon "github.com/keptn/go-utils/pkg/lib/keptn"
	"github.com/keptn/go-utils/pkg/sdk"
	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"

	"github.com/Mouhamadou305/splunk-sli-provider/utils"

	keptnv2 "github.com/keptn/go-utils/pkg/lib/v0_2_0"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/client-go/kubernetes/typed/core/v1"
)

// GetSliEventHandler is responsible for processing configure monitoring events
type GetSliEventHandler struct {
	kubeClient kubernetes.Clientset
}

// NewGetSliEventHandler creates a new TriggeredEventHandler
func NewGetSliEventHandler(kubeClient kubernetes.Clientset) *GetSliEventHandler {
	return &GetSliEventHandler{
		kubeClient: kubeClient,
	}
}

type splunkCredentials struct {
	Host     string `json:"host" yaml:"host"`
	Token	 string `json:"token" yaml:"token"`
}

var env utils.EnvConfig

// Execute processes an event
func (eh GetSliEventHandler) Execute(k sdk.IKeptn, event sdk.KeptnEvent) (interface{}, *sdk.Error) {
	if err := envconfig.Process("", &env); err != nil {
		k.Logger().Error("Failed to process env var: " + err.Error())
	}

	eventData := &keptnv2.GetSLITriggeredEventData{}
	if err := keptnv2.Decode(event.Data, eventData); err != nil {
		return nil, &sdk.Error{Err: err, StatusType: keptnv2.StatusErrored, ResultType: keptnv2.ResultFailed, Message: "failed to decode get-sli.triggered event: " + err.Error()}
	}

	// get splunk API URL for the provided Project from Kubernetes Config Map
	splunkCreds, err := getSplunkAPIURL(eventData.Project, eh.kubeClient.CoreV1())
	if err != nil {
		return nil, &sdk.Error{Err: err, StatusType: keptnv2.StatusErrored, ResultType: keptnv2.ResultFailed, Message: "failed to get Prometheus API URL: " + err.Error()}
	}

	// determine deployment type based on what lighthouse-service is providing
	deployment := eventData.Deployment // "canary", "primary" or "" (or "direct" or "user_managed")
	// fallback: get deployment type from labels
	if deploymentLabel, ok := eventData.Labels["deployment"]; deployment == "" && !ok {
		log.Println("Warning: no deployment type specified in event, defaulting to \"primary\"")
		deployment = "primary"
	} else if ok {
		log.Println("Deployment was not set, but label exist. Using label from event")
		deployment = deploymentLabel
	}

	// get SLI queries (from SLI.yaml)
	projectCustomQueries, err := getCustomQueries(k.GetResourceHandler(), eventData.Project, eventData.Stage, eventData.Service)
	if err != nil {
		return nil, &sdk.Error{Err: err, StatusType: keptnv2.StatusErrored, ResultType: keptnv2.ResultFailed, Message: fmt.Sprintf("unable to retrieve custom queries for project %s: %e", eventData.Project, err)}
	}

	// retrieve metrics from splunk
	sliResults := retrieveMetrics(splunkCreds, deployment, projectCustomQueries, eventData)

	// If we hand any problem retrieving an SLI value, we set the result of the overall .finished event
	// to Warning, if all fail ResultFailed is set for the event
	finalSLIEventResult := keptnv2.ResultPass

	if len(sliResults) > 0 {
		sliResultsFailed := 0
		for _, sliResult := range sliResults {
			if !sliResult.Success {
				sliResultsFailed++
			}
		}

		if sliResultsFailed > 0 && sliResultsFailed < len(sliResults) {
			finalSLIEventResult = keptnv2.ResultWarning
		} else if sliResultsFailed == len(sliResults) {
			finalSLIEventResult = keptnv2.ResultFailed
		}
	}

	// construct finished event data
	getSliFinishedEventData := &keptnv2.GetSLIFinishedEventData{
		EventData: keptnv2.EventData{
			Status:  keptnv2.StatusSucceeded,
			Result:  finalSLIEventResult,
			Project: eventData.Project,
			Stage:   eventData.Stage,
			Service: eventData.Service,
			Labels:  eventData.Labels,
		},
		GetSLI: keptnv2.GetSLIFinished{
			IndicatorValues: sliResults,
			Start:           eventData.GetSLI.Start,
			End:             eventData.GetSLI.End,
		},
	}

	if getSliFinishedEventData.EventData.Result == keptnv2.ResultFailed {
		getSliFinishedEventData.EventData.Message = "unable to retrieve metrics"
	}

	return getSliFinishedEventData, nil
}

func retrieveMetrics(splunkCreds *splunkCredentials, deployment string, projectCustomQueries map[string] string, eventData *keptnv2.GetSLITriggeredEventData) []*keptnv2.SLIResult {
	log.Printf("Retrieving Prometheus metrics")

	var sliResults []*keptnv2.SLIResult

	for _, indicator := range eventData.GetSLI.Indicators {
		log.Println("retrieveMetrics: Fetching indicator: " + indicator)

		cmd := exec.Command("python", "-c", "import splunk; "+
		"sp= splunk.SplunkProvider(project="+eventData.Project+
								",stage="+eventData.Stage+
								",service="+eventData.Service+
								", deploymentType="+deployment+
								", labels="+getMapContent(eventData.Labels)+
								", customQueries="+getMapContent(projectCustomQueries)+
								", host="+splunkCreds.Host+
								", token="+splunkCreds.Token+");"+
		"print(sp.get_sli("+indicator+", "+ eventData.GetSLI.Start+","+ eventData.GetSLI.End+"))")
		
		out, err := cmd.CombinedOutput()
		sliValue, _:= strconv.ParseFloat(string(out), 8)

		if err != nil {
			sliResults = append(sliResults, &keptnv2.SLIResult{
				Metric:  indicator,
				Value:   0,
				Success: false,
				Message: err.Error(),
			})
		} else {
			sliResults = append(sliResults, &keptnv2.SLIResult{
				Metric:  indicator,
				Value:   sliValue,
				Success: true,
			})
		}
	}

	return sliResults
}

func getMapContent(mp map[string] string) string{
	dictn :="{"
	for key, element := range mp {
		dictn= dictn+"\""+key+"\""+" : "+"\""+element+"\""+","
	}
	dictn=strings.TrimSuffix(dictn, ",")+"}"
	return dictn
}

func getCustomQueries(resourceHandler sdk.ResourceHandler, project string, stage string, service string) (map[string]string, error) {
	log.Println("Checking for custom SLI queries")

	customQueries, err := GetSLIConfiguration(resourceHandler, project, stage, service, utils.SliResourceURI)
	if err != nil {
		return nil, err
	}

	return customQueries, nil
}

// getSplunkAPIURL fetches the splunk API URL for the provided project (e.g., from Kubernetes configmap)
func getSplunkAPIURL(project string, kubeClient v1.CoreV1Interface) (*splunkCredentials, error) {
	log.Println("Checking if external splunk instance has been defined for project " + project)

	secretName := fmt.Sprintf("splunk-credentials-%s", project)

	secret, err := kubeClient.Secrets(env.PodNamespace).Get(context.TODO(), secretName, metav1.GetOptions{})

	// fallback: return cluster-internal splunk URL (configured via SplunkEndpoint environment variable)
	// in case no secret has been created for this project
	if err != nil {
		log.Println("Could not retrieve or read secret (" + err.Error() + ") for project " + project + ". Using default: " + env.SplunkEndpoint)
		return nil, nil //attention : ecouter audio
	}

	pc := splunkCredentials{}

	// Read Splunk config from Kubernetes secret as strings
	// Example: keptn create secret splunk-credentials-<project> --scope="keptn-splunk-sli-provider" --from-literal="SPLUNK_HOST=$SPLUNK_HOST"
	splunkHost, errHost := utils.ReadK8sSecretAsString(env.PodNamespace, secretName, "SPLUNK_HOST")
	splunkToken, errToken := utils.ReadK8sSecretAsString(env.PodNamespace, secretName, "SPLUNK_TOKEN")

	if errHost == nil && errToken == nil{
		// found! using it
		pc.Host = strings.Replace(splunkHost, " ", "", -1)
		pc.Token = splunkToken
	} else {
		// deprecated: try to use legacy approach
		err = yaml.Unmarshal(secret.Data["splunk-credentials"], &pc)

		if err != nil {
			log.Println("Could not parse credentials for external splunk instance: " + err.Error())
			return nil, errors.New("invalid credentials format found in secret 'splunk-credentials-" + project)
		}

		// warn the user to migrate their credentials
		log.Printf("Warning: Please migrate your splunk credentials for project %s. ", project)
		log.Printf("See https://github.com/Mouhamadou305/splunk-sli-provider/issues/274 for more information.\n")
	}

	log.Println("Using external splunk instance for project " + project + ": " + pc.Host)
	return &pc, nil
}

// GetSLIConfiguration retrieves the SLI configuration for a service considering SLI configuration on stage and project level.
// First, the configuration of project-level is retrieved, which is then overridden by configuration on stage level,
// overridden by configuration on service level.
func GetSLIConfiguration(resourceHandler sdk.ResourceHandler, project string, stage string, service string, resourceURI string) (map[string]string, error) {
	var res *models.Resource
	var err error
	SLIs := make(map[string]string)

	// get sli config from project
	if project != "" {
		scope := api.NewResourceScope()
		scope.Project(project)
		scope.Resource(resourceURI)
		res, err = resourceHandler.GetResource(*scope)
		if err != nil {
			// return error except "resource not found" type
			if !strings.Contains(strings.ToLower(err.Error()), "resource not found") {
				return nil, err
			}
		}
		SLIs, err = addResourceContentToSLIMap(SLIs, res)
		if err != nil {
			return nil, err
		}
	}

	// get sli config from stage
	if project != "" && stage != "" {
		scope := api.NewResourceScope()
		scope.Project(project)
		scope.Stage(stage)
		scope.Resource(resourceURI)
		res, err = resourceHandler.GetResource(*scope)
		if err != nil {
			// return error except "resource not found" type
			if !strings.Contains(strings.ToLower(err.Error()), "resource not found") {
				return nil, err
			}
		}
		SLIs, err = addResourceContentToSLIMap(SLIs, res)
		if err != nil {
			return nil, err
		}
	}

	// get sli config from service
	if project != "" && stage != "" && service != "" {
		scope := api.NewResourceScope()
		scope.Project(project)
		scope.Stage(stage)
		scope.Service(service)
		scope.Resource(resourceURI)
		res, err = resourceHandler.GetResource(*scope)
		if err != nil {
			// return error except "resource not found" type
			if !strings.Contains(strings.ToLower(err.Error()), "resource not found") {
				return nil, err
			}
		}
		SLIs, err = addResourceContentToSLIMap(SLIs, res)
		if err != nil {
			return nil, err
		}
	}

	return SLIs, nil
}

func addResourceContentToSLIMap(SLIs map[string]string, resource *models.Resource) (map[string]string, error) {
	if resource != nil {
		sliConfig := keptncommon.SLIConfig{}
		err := yaml.Unmarshal([]byte(resource.ResourceContent), &sliConfig)
		if err != nil {
			return nil, err
		}

		for key, value := range sliConfig.Indicators {
			SLIs[key] = value
		}

		if len(SLIs) == 0 {
			return nil, errors.New("missing required field: indicators")
		}
	}
	return SLIs, nil
}
