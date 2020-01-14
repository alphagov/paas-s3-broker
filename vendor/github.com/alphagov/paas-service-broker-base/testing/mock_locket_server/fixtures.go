package mock_locket_server

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager"
)

const (
	LocketServerKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEAns+gRhw/279kiAjPrwQlqi14MDixBs8Z/0T2U8E9/aPdjK5L
1qRDpwLo7pkIoeuh9Wx4hCoLthmn0gQKQPzyeHofqpszryLch3CSqvLbF1AifE4C
80F2oBq1q51n7OtksEk6nQfPlWE5VHGahU1zm8WaxB4ALN46T+e1nqhGMB7R/sTn
A0aIwYDHBfIELls8rjFZXLpbXgrGdMSl4e1jChEcJkLxQkgFKcMoIznwd2doCpg8
PhzOUeONRGZ9eqJVa+3Y0XtCHyZ0w5LgOD5Y8dPb2lfc0fG6+QXUxSbuZ5AazEIY
tYif5tYY0j1/ijxUZJHXf7MZw20oqw1osP+PrQIDAQABAoIBAD/yUJIKi/gbEArq
qV3KqLPmjS+1lCqut8Qe66T7+c8o7WvZPvZVvFwCgvKYMm6op9Vf8QMevwp7OCUL
tWlHsQar/hY/WkdaHTx4Ksak9W1kug4dh3iV0oNTXfWVcyRmAGwvgGd8nqyCsof7
GoI0lqmRnuj7P4zRit9j6LDTBzgWfoO8xu2yYI7TLNDgZ8BX3BGf0J0rbce/D62O
E1WUT7LD3wOABmb4JsJBSWenRFn+FOvm0bTFFjETqlJVAJChlpMTjLRPInFishFx
AfyGpIjgEqqgBRvsvT+EFMuHNRIrG7KAgCpPr04UCEW3wM3rYsq8+6201q2auyNs
80iXqw0CgYEAzef6ETbkSiJuARolES4xL1ARmy92UZZH/pDJwWqvYRjNJBwIkexC
5HRTEqJvetqKjuyHRjlsG3XrI7jO+r/k+HJwvacDDKAmH1OjgtL+BodA7b8DOT86
zQbvs7NLY+xno29YS2u7LnRwXMmOBCt9cO5egT+qs8AdVYdd9nsNAvMCgYEAxXKF
swQBo7Q25LsjXpPdro4TxP8tBKgPOvo1G/pzq7+K08Yi4KQPLj9WVz9XYFOCVqiM
+4hQyP7RWiZZVANPvWB0O1h0NHF/A81JgKJVGdXBKO1K686wDL5vYB7T63pU7WOk
1LiZZqRpnIv/hRf8EN72xPsBTaY+/H5IP+Vhit8CgYBncqrCR0++pzmZOCdzUD/J
w3J1Aw1wxA37qYaTtCPUpn86KxNrLMYWvRKXhCB6Gp4OXGtCLstPqJiwY8MpW4uP
/v8BaY0wpK1Cg+Tcb2DMqttGFvdppYjHRTrcj7HKzBTtmZ1ElyV9m2ZwV5sQIUFu
oXO9f90lXdnfBJmCoiPRXQKBgQCFolEjLB7/8UUF4jK6HFH5hmeS+TI66JQGUroH
SadoIqePVZbde6xanLuPKWu14k9g34sr4sLqhqyi2zmyRtt9TP7d+6wKopZYuGR7
D2ORrL6jOJdwqd81gN5YrAS6Z317feldn+MTOUvRjF9QcT9FG+LgxxHGwDH5Km8z
78fo+QKBgDDB/hIy8rmq5R6lJQyNXKGE8HDRyHEGijPWJ4/W+UY97WcOmeK6e7aJ
2jrt/QPcUugWtkPsStU46/EQUUHxCGEroLMr2C/xhPxRSnt+gq/6+FbI2XcK975z
ItMPdTWaVNW957do2O22Ro6cZuFSZMqwqvbwQklr+YswgPBPNHh4
-----END RSA PRIVATE KEY-----`
	LocketServerCertPEM = `-----BEGIN CERTIFICATE-----
MIIDjTCCAnWgAwIBAgIJAPsBhYKcr8vaMA0GCSqGSIb3DQEBCwUAMF0xCzAJBgNV
BAYTAlVLMQswCQYDVQQIDAJFTjEPMA0GA1UEBwwGTG9uZG9uMQwwCgYDVQQKDANH
RFMxDTALBgNVBAsMBFBhYVMxEzARBgNVBAMMCkZha2VMb2NrZXQwHhcNMTkwOTEz
MTAyODU1WhcNNDcwMTI5MTAyODU1WjBdMQswCQYDVQQGEwJVSzELMAkGA1UECAwC
RU4xDzANBgNVBAcMBkxvbmRvbjEMMAoGA1UECgwDR0RTMQ0wCwYDVQQLDARQYWFT
MRMwEQYDVQQDDApGYWtlTG9ja2V0MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAns+gRhw/279kiAjPrwQlqi14MDixBs8Z/0T2U8E9/aPdjK5L1qRDpwLo
7pkIoeuh9Wx4hCoLthmn0gQKQPzyeHofqpszryLch3CSqvLbF1AifE4C80F2oBq1
q51n7OtksEk6nQfPlWE5VHGahU1zm8WaxB4ALN46T+e1nqhGMB7R/sTnA0aIwYDH
BfIELls8rjFZXLpbXgrGdMSl4e1jChEcJkLxQkgFKcMoIznwd2doCpg8PhzOUeON
RGZ9eqJVa+3Y0XtCHyZ0w5LgOD5Y8dPb2lfc0fG6+QXUxSbuZ5AazEIYtYif5tYY
0j1/ijxUZJHXf7MZw20oqw1osP+PrQIDAQABo1AwTjAdBgNVHQ4EFgQUmvdezVJz
V34Mw61GxvnJAoipzMwwHwYDVR0jBBgwFoAUmvdezVJzV34Mw61GxvnJAoipzMww
DAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEALn3NYjnEBbA+05cA1PRC
1OmuStGTqG1AoNtb7g782/ZmniweiJr1P1hPdiAF0n0BLkyt9R1UJ0rfzuvuZCgp
J9B7ivIBjetNhC5pzlrNiEV0ZInk3EdRlkrQr411R1h5BzKl7ZM3o53/hVxc5EXV
4vDxcQ8oYRmdIE0nH+6SFyUjRY4KkAg7Lxx/W7N979zdXOmKE2yTXbCDpnEyETuR
Itvgv4b6MMD5WQEXtCztsQL9THlITy9oOkLlyTretP8tIbNtH+xE1c9vXmulW/9t
SeKyVF5d5atxZNHwShgIlvmKQ5Wkw3CH8yPjeY5OL77orlYWzzf9S9aAQt++PyW2
zA==
-----END CERTIFICATE-----`
	LocketClientKeyPEM = `-----BEGIN RSA PRIVATE KEY-----
MIIEowIBAAKCAQEApUGLwiv4u7RK0vf0wLFOrffy6B3e8HovfvsJjHXZWjIHCaLh
zt6bjscB8QFW2zii4PNxrcCjfkHxX3yRw2leSincGZXEQoA1f5FDEwlNsYpmj9Uu
FvnS+tYurgjWyjPC9WRqAESw5bnGvgkNWXJ4pzgQT2aNxD/Ux+MWJ4phzFidSF6i
8Z+9YfgMBBg48L52fZbVzuRZaA+YNvbYSYs+PoQlZxzRReNoRy9KB4El0n6r0OaU
7Qs+mWOgNGJf07SaxAyAaSl7ixR3KV2AFUPnnEghPGX0rUl1y5R8N3k1jOz2CDi4
libTmjvMvlkSKr//zSxe9vrD/r/PlbMgSm3QFwIDAQABAoIBAGPIlC8GhgYw8w04
E11Wsn9xPAbUIo1p+cveoQFjF37SvlUfOOHEoIExwQZZHKz6Ib9av8V+kDnob2qM
uScZNInrhK8eM4dECwmgGLQh5fvR6ePngRD2gGJdeYo0ZB1r68hofWj0ESxlZI/S
v3DHIfs2awLGlctuD3kysWnmsO7FinHpTx/qA7YHWqWXQ+Vm8TE4n+j44Pl1OnOB
Q50ikOjb99i4CWFfl2oUH+X5xXkevES/sCBmCAsGCElQo5hias21oyMgrhEKuzIZ
/tX8mk7ZBmqkqwr3gCAimEG2qW1cRD+R4mtigjsFFIPgZPIqZ2hjmKZ3+9+F6J+U
WKMREAECgYEA1uAprqKVMPePqMHHDjr6esm+CrM3F86u85Ao2BcmMirM/iajlEh5
HsT8P9bf5uAlAQ96VL7X22AEWJPtGsoiq/iL9AAH0buep4lQolrga7B3uk2g8RDC
BplZJWQ2ejw4m2r8J74W46k2jdUldsF81rL1DevEdJzHKYt3asTvDgECgYEAxOJF
CwVPg0fk4IXehcrWPEpIhb9rq/Ag4hQcSLbYKQLWiZReAI7qI8TwV6A53w3bJV0S
lOacfyNw6Aqz3CbZg3Tw7SItove+zuUvgltfUOQ16K54sTjIeAXImTkru5LpmSJB
ftNSHrDti6fUqNMg/i4zEOhTdUunZOyScm0vjhcCgYBTIm9+DZFDXMTMOgzVyKPY
le1dHnGWWHT/7yqeUHaKulyNiE2JtXCHIxela3E9VkN64Y4m8594VPHZg4Ic90/q
0UL0qH5d+wUrNMlpx1dE0wW/owE9w4oOG46OFPOu31XXa9EbX0Rj2Lgur+TKyZmP
R7XgKPPdWjsEK92MBZ2oAQKBgFgsFUuYN0HN4ryCd2NnsYYSpmPvlCLOSYu2Aey2
phvHv5ihr2+EkWsveYtkoEY6iFg1VGsG1DNEBf6FPINtiqAKsRMh6VpApV022o4A
qbEqYtIvwLFtgqntvSaRqfo5ExCXfMl1jiNcjSWsJdrtoqryub/qq+Wt2eui3vsL
1u5FAoGBAL5ERKv8zP9ASD18qblz5c9YWYbOZjNGlIfzS8ofwNxTgkibfygZ16dZ
CqvtVhdzJpoF2vS5pTwk2sqNNdx7D+b6LBFmNB/CrtmdJNk81veidErliGN+wqhN
vTG67JlcvGnz6bF+BMnfgc/KT+pox2DaeyiFxoaSZrmi5tJVJUYU
-----END RSA PRIVATE KEY-----`
	LocketClientCertPEM = `-----BEGIN CERTIFICATE-----
MIIDmTCCAoGgAwIBAgIJANI92EIzO/J9MA0GCSqGSIb3DQEBCwUAMGMxCzAJBgNV
BAYTAlVLMQswCQYDVQQIDAJFTjEPMA0GA1UEBwwGTG9uZG9uMQwwCgYDVQQKDANH
RFMxDTALBgNVBAsMBFBhYVMxGTAXBgNVBAMMEEZha2VMb2NrZXRDbGllbnQwHhcN
MTkwOTEzMTAzMDA3WhcNNDcwMTI5MTAzMDA3WjBjMQswCQYDVQQGEwJVSzELMAkG
A1UECAwCRU4xDzANBgNVBAcMBkxvbmRvbjEMMAoGA1UECgwDR0RTMQ0wCwYDVQQL
DARQYWFTMRkwFwYDVQQDDBBGYWtlTG9ja2V0Q2xpZW50MIIBIjANBgkqhkiG9w0B
AQEFAAOCAQ8AMIIBCgKCAQEApUGLwiv4u7RK0vf0wLFOrffy6B3e8HovfvsJjHXZ
WjIHCaLhzt6bjscB8QFW2zii4PNxrcCjfkHxX3yRw2leSincGZXEQoA1f5FDEwlN
sYpmj9UuFvnS+tYurgjWyjPC9WRqAESw5bnGvgkNWXJ4pzgQT2aNxD/Ux+MWJ4ph
zFidSF6i8Z+9YfgMBBg48L52fZbVzuRZaA+YNvbYSYs+PoQlZxzRReNoRy9KB4El
0n6r0OaU7Qs+mWOgNGJf07SaxAyAaSl7ixR3KV2AFUPnnEghPGX0rUl1y5R8N3k1
jOz2CDi4libTmjvMvlkSKr//zSxe9vrD/r/PlbMgSm3QFwIDAQABo1AwTjAdBgNV
HQ4EFgQU6i6gqoFK50RhJZQiWLvtezq230gwHwYDVR0jBBgwFoAU6i6gqoFK50Rh
JZQiWLvtezq230gwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAh2/6
sFgHfdEZlohMx9QZM5m04VPc4xmD3iLaNv/6Iv+PgB92oSxzspk2+8A96l8J4+29
58Wcst6gXFMDrmINrrjp5IANeiY3D/bO9AmLHWWoVP/DMkQ8h409pvMjH1fT47r+
pBecNP/aimBHeGe2gxnFCqwrHQDyEveN+gL61Eyiobi2pfUQSa/gDXCZdRJLjr18
4nszwOKo+WkzYBQUEWWWaeoRVKDeEfj7jIb+U4Dgqn2xpVhEr/FFJgFNjxBt+9OM
+LL74pPrvVjgOSDAXILu1q7/d0lpg5xxb2pSGGcvl4SsUJqvKcZfM1qHfheBZb/X
yJ+DHjRh1Z/TDPo5LA==
-----END CERTIFICATE-----`
)

type LocketFixtures struct {
	Filepath string
}

func SetupLocketFixtures() (LocketFixtures, error) {
	logger := lager.NewLogger("locket-fixtures")
	logger.RegisterSink(lager.NewWriterSink(os.Stdout, lager.DEBUG))
	f := LocketFixtures{}
	filepath, err := ioutil.TempDir("", "locket-fixtures")
	if err != nil {
		logger.Error("Failed to create temp directory", err)
		return LocketFixtures{}, err
	}
	f.Filepath = filepath
	err = ioutil.WriteFile(f.Filepath+"/locket-client.cert.pem", []byte(LocketClientCertPEM), 0644)
	if err != nil {
		logger.Error("Failed to create locket-client.cert.pem", err)
		return LocketFixtures{}, err
	}
	err = ioutil.WriteFile(f.Filepath+"/locket-client.key.pem", []byte(LocketClientKeyPEM), 0644)
	if err != nil {
		logger.Error("Failed to create locket-client.key.pem", err)
		return LocketFixtures{}, err
	}
	err = ioutil.WriteFile(f.Filepath+"/locket-server.cert.pem", []byte(LocketServerCertPEM), 0644)
	if err != nil {
		logger.Error("Failed to create locket-server.cert.pem", err)
		return LocketFixtures{}, err
	}
	err = ioutil.WriteFile(f.Filepath+"/locket-server.key.pem", []byte(LocketServerKeyPEM), 0644)
	if err != nil {
		logger.Error("Failed to create locket-server.key.pem", err)
		return LocketFixtures{}, err
	}
	return f, nil
}

func (f LocketFixtures) Cleanup() {
	os.RemoveAll(f.Filepath)
}
