package temporal

import "go.temporal.io/sdk/client"

type ClientConfig struct {
	HostPort  string
	Namespace string
}

func NewClient(config ClientConfig) (client.Client, error) {
	options := client.Options{
		HostPort:  config.HostPort,
		Namespace: config.Namespace,
	}
	return client.Dial(options)
}
