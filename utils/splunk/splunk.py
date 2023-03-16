from logging import error
from typing import Dict, Tuple
import math
from utils import *
from datetime import datetime
import splunklib.client as client
import splunklib.results as results

class SplunkProvider:
    def __init__(
        self,
        project,
        stage,
        service,
        deploymentType: str,
        labels: Dict[str, str] = None,
        customFilters: Dict[str, str] = None,
        customQueries: Dict[str, str] = None,
        host: str="",
        username: str="",
        password: str="",
        token: str="",
        autologin= True,
    ) -> None:
        if(username!="" and password!=""):
            #connecting using username and password
            self.service = client.connect(host=host, username=username, password=password, autologin=autologin)
        elif(token!="" and host!=""):
            #connecting using bearer token
            self.service = client.connect(host=host, splunkToken=token, autologin=autologin)
        else:
            error("Connection credentials are invalid")

        self.project = project
        self.stage = stage
        self.service = service
        self.deployment_type = deploymentType
        self.labels = labels
        self.custom_filters = customFilters
        self.custom_queries = customQueries

    def get_sli(
        self, metric: str, start_time: str, end_time: str
    ) -> Tuple[float, None]:
        start_unix, end_unix = self._parse_unix_timestamps(start_time, end_time)

        kwargs_oneshot = {"earliest_time": start_time,
                  "latest_time": end_time,
                  "output_mode": 'json'}
        searchquery_oneshot = self._get_metric_query(metric, start_unix, end_unix)

        oneshotsearch_results = self.service.jobs.oneshot(searchquery_oneshot, **kwargs_oneshot)

        sli = 0.0
        for result in results.JSONResultsReader(oneshotsearch_results):
            try:
                sli_value = float(result[metric])
            except ValueError as e:
                raise ValueError(f"failed to parse {metric}: {e}")
            sli = sli_value

        return sli

    def _get_metric_query(self, metric: str, start_time: str, end_time: str) -> str:
        query = self.custom_queries.get(metric, None)
        if query is not None:
            return self._replace_query_parameters(query, start_time, end_time)
        raise ValueError(f"No Custom query specified for metric {metric}")

    def _replace_query_parameters(self, query: str, start_time: str, end_time: str) -> str:
        for filter in self.custom_filters:
            filter_value = filter.value.replace("'", "").replace('"', "")
            query = query.replace("$" + filter.key, filter_value).replace(
                "$" + filter.key.upper(), filter_value
            )
        query = query.replace("$PROJECT", self.project)
        query = query.replace("$STAGE", self.stage)
        query = query.replace("$SERVICE", self.service)
        query = query.replace("$DEPLOYMENT", self.deployment_type)

        # replace labels
        for key, value in self.labels.items():
            query = query.replace(f"$LABEL.{key}", value)

        # replace duration
        duration_seconds = str(
            math.floor((end_time - start_time).total_seconds())
        ) + "s"

        query = query.replace("$DURATION_SECONDS", duration_seconds)
        return query

    def _parse_unix_timestamps(self, start_time: str, end_time: str) -> Tuple[int, int]:
        start_unix = int(datetime.datetime.fromisoformat(start_time).timestamp())
        end_unix = int(datetime.datetime.fromisoformat(end_time).timestamp())

        return start_unix, end_unix
    
