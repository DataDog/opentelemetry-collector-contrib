This is the POC for the Datadog Fleet Automation Extension.

You can configure this extension in service, using the following configuration values:
`api::key`: a Datadog API Key, required
`api::site`: your Datadog site value (e.g. us5.datadoghq.com), defaults to "datadoghq.com"
`hostname`: custom hostname; if you do not specify one, the extension will try to infer one. Note: this must match any hostname value set in the `host_metadata` section of Datadog Exporter, if enabled in your collector.
`reporter_period`: A value (given in time notation, e.g. "20m") of time between sending fleet automation data to Datadog backend. Must be 5 minutes or greater.
