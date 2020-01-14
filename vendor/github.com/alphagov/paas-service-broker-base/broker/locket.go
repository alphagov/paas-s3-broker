package broker

type LocketConfig struct {
	Address        string `json:"address"`
	CACertFile     string `json:"ca_cert_file"`
	ClientCertFile string `json:"client_cert_file"`
	ClientKeyFile  string `json:"client_key_file"`
	SkipVerify     bool
}

type ServiceLock struct {
	ServiceName string
	Key         string
	Owner       string
}
