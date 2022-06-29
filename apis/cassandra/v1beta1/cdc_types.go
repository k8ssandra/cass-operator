package v1beta1

// CDCConfiguration holds CDC config for the CassandraDatacenter. Note that it cannot contain arrays, channels, maps etc. because of the way the
// reflection logic works which marshalls it into a string for the purposes of passing on the command line.
type CDCConfiguration struct {
	Enabled bool `json:"enabled"`
	// +kubebuilder:validation:MinLength=1
	PulsarServiceUrl *string `json:"pulsarServiceUrl"`
	// +optional
	TopicPrefix *string `json:"topicPrefix,omitempty"`
	// +optional
	CDCWorkingDir *string `json:"cdcWorkingDir,omitempty"`
	// +optional
	CDCPollIntervalMs *int `json:"cdcPollIntervalM,omitempty"`
	// +optional
	ErrorCommitLogReprocessEnabled *bool `json:"errorCommitLogReprocessEnabled,omitempty"`
	// +optional
	CDCConcurrentProcessors *int `json:"cdcConcurrentProcessors,omitempty"`
	// +optional
	PulsarBatchDelayInMs *int `json:"pulsarBatchDelayInMs,omitempty"`
	// +optional
	PulsarKeyBasedBatcher *bool `json:"pulsarKeyBasedBatcher,omitempty"`
	// +optional
	PulsarMaxPendingMessages *int `json:"pulsarMaxPendingMessages,omitempty"`
	// +optional
	PulsarMaxPendingMessagesAcrossPartitions *int `json:"pulsarMaxPendingMessagesAcrossPartitions,omitempty"`
	// +optional
	PulsarAuthPluginClassName *string `json:"pulsarAuthPluginClassName,omitempty"`
	// +optional
	PulsarAuthParams *string `json:"pulsarAuthParams,omitempty"`
	// +optional
	SSLProvider *string `json:"sslProvider,omitempty"`
	// +optional
	SSLTruststorePath *string `json:"sslTruststorePath,omitempty"`
	// +optional
	SSLTruststorePassword *string `json:"sslTruststorePassword,omitempty"`
	// +optional
	SSLTruststoreType *string `json:"sslTruststoreType,omitempty"`
	// +optional
	SSLKeystorePath *string `json:"sslKeystorePath,omitempty"`
	// +optional
	SSLKeystorePassword *string `json:"sslKeystorePassword,omitempty"`
	// +optional
	SSLCipherSuites *string `json:"sslCipherSuites,omitempty"`
	// +optional
	SSLEnabledProtocols *string `json:"sslEnabledProtocols,omitempty"`
	// +optional
	SSLAllowInsecureConnection *string `json:"sslAllowInsecureConnection,omitempty"`
	// +optional
	SSLHostnameVerificationEnable *string `json:"sslHostnameVerificationEnable,omitempty"`
}
