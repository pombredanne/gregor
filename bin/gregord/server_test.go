package main

import (
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeTmp(d string) (string, error) {
	f, err := ioutil.TempFile("", "gregor_test")
	if err != nil {
		return "", err
	}
	_, err = f.WriteString(d)
	return f.Name(), err

}

const caCert = `-----BEGIN CERTIFICATE-----
MIIFgDCCA2gCCQDF4YJuQAWDqTANBgkqhkiG9w0BAQsFADCBgTELMAkGA1UEBhMC
VVMxCzAJBgNVBAgMAk1BMQ8wDQYDVQQHDAZCb3N0b24xEzARBgNVBAoMCkV4YW1w
bGUgQ28xEDAOBgNVBAsMB3RlY2hvcHMxCzAJBgNVBAMMAmNhMSAwHgYJKoZIhvcN
AQkBFhFjZXJ0c0BleGFtcGxlLmNvbTAeFw0xNjAyMjUyMTQ3MzhaFw00MzA3MTIy
MTQ3MzhaMIGBMQswCQYDVQQGEwJVUzELMAkGA1UECAwCTUExDzANBgNVBAcMBkJv
c3RvbjETMBEGA1UECgwKRXhhbXBsZSBDbzEQMA4GA1UECwwHdGVjaG9wczELMAkG
A1UEAwwCY2ExIDAeBgkqhkiG9w0BCQEWEWNlcnRzQGV4YW1wbGUuY29tMIICIjAN
BgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEA8NScIAfl3DK26CwnMSH1TXKurE/B
BOocNApkH/913F28AgxzsS+blsG1IyjSuG9ls5shqlGpWQs1kM9PqFz6Yl5Y3H8b
cwY0dWk1RmrZ6EWV/lWuLZxiKB8rBJksUVvdcuhnNpvOjYvkTgL9q7OObMdz3lvH
2pqwa8TWgw9EITKCam7i4860qcOoVkhCFitrihg182UmXWmuAZOm5N0R9+Y5t8yQ
7S3XKYZLtKND7ZGD51AfjN6TN1jN8kd9KMii7JITtvqsJDOxl0Kzn9fefgnCQF1G
P7ilLybId4W5pCO/8mKXb0CQlJ9kAYVfxWPNR87ZQA9KLC8nXu3xWaplXZl7T4Tq
wZHD85lbpLurSkJliizwDgs3cootEXs04ssl6SpVnc/Qxat3jomCtmKBtY5Cxvy9
IwHmaYWCYAIiPcru8U1cVg3xsH6i2JTz7uZRFvEjhYNqr1o6QnKcJ6cYGs13tYwA
57Xl1CVJ8hBMmtlzqbA2xMCbmkpWitjzXyArzQjAD0dDeGmStGOOQqy/N4LJaQ6+
+q2bHpx5Cd6DxNf868iWupuKadT923ZDzAn1PhDWugKQ2BSIzM2O57m1HYmGm3be
NpwTYKuZGCDaLwDhnbIICTgQXjyCDTV4TfOKBPzr+i+yAjdjJimXHQ5gy7BMJoO6
fOWYqbs8vgvx4WUCAwEAATANBgkqhkiG9w0BAQsFAAOCAgEAhLLxyfdQdQDdo3YG
s1fKqm5lLu0Dx6uzNtIVY0n7vyyAolBVDlJ7Du84b344/U4kRgjNwAx2ZECvWkEZ
ov6+VMYX6EkV/0nwRNOoADYO8YVlZzBvwZgA12Vkw9NHje18FnQcS3L4nFjJPFoY
UEBhK5qTXqxJ9PK9aBZXIhDT2u/o9xEecuC3kjqNI6bi5zsZ5y04Qulr/1UwWy2e
IFFfySdL7kzZkhQAawg/+pNgentVykRRNgCVmFQ4uytTpp45pAtSNBaLm8RCrNGF
AybVh7HAW+LwjUOPpYQ38j1neiFS8NFJRKNKS2OtbS743NnYWbYOJdGWH4jwOluL
PjckYdTGO82EIjxcGXIF5UPw6W3ozwCqGgO1bCY8tgcjoUPm3hUrTzZ5ueXRUkNI
qwPrmvpLUtJjI7prCAsi3gDoL/+t7LNEAYPYreRc+LdvJTRj90WwWCdXTHfSMVjt
NN9Mt339LkwXGCb6CavmDgE7oVbrFPSTbeFFPhaheQh7pjLFhl9ZBfE7g3d9oNOX
PmyY3I0kAE41RiDMrrxHO3tHv9IaQUUDDcGzIWFJlnbvQRXAsWf/HH56Q0eIAZZp
K++p6Mo0K+KCu0IwKwdcTYKqty6xefK83p0j/IWVW29Lka44f+ZAroUlBn1+W4GO
sB31+boS8zC7SOmgWuaHeOQdLT8=
-----END CERTIFICATE-----
`

const serverCrt = `-----BEGIN CERTIFICATE-----
MIIFjDCCA3SgAwIBAgIJANZcjCFW4EqWMA0GCSqGSIb3DQEBCwUAMIGBMQswCQYD
VQQGEwJVUzELMAkGA1UECAwCTUExDzANBgNVBAcMBkJvc3RvbjETMBEGA1UECgwK
RXhhbXBsZSBDbzEQMA4GA1UECwwHdGVjaG9wczELMAkGA1UEAwwCY2ExIDAeBgkq
hkiG9w0BCQEWEWNlcnRzQGV4YW1wbGUuY29tMB4XDTE2MDIyNTIxNDczOFoXDTE4
MTEyMDIxNDczOFowgYgxCzAJBgNVBAYTAlVTMQswCQYDVQQIDAJNQTEPMA0GA1UE
BwwGQm9zdG9uMRMwEQYDVQQKDApFeGFtcGxlIENvMRAwDgYDVQQLDAd0ZWNob3Bz
MRIwEAYDVQQDDAlsb2NhbGhvc3QxIDAeBgkqhkiG9w0BCQEWEWNlcnRzQGV4YW1w
bGUuY29tMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAySPS4gbmUAdx
H2WmnaKTehUTfFBs+dVR9WeYD8IFM6fuVa/ajABDNp0cMFnM0msbTvPIPQ7Sb+ED
hM9LqD42hohIca94C0IZjipMz45BZFGiFWsrUoJix2BvzepHntcpqPUu/5Ldn97f
3VA995NUHAy26I7YxlPkJbtEyQkH3jtQjd1J1IoKi0wFNQj+qG8OtpFauS0uge60
N54nRRMik8fR6iT7Mj22qCcvmOMl8ucJ3Qx18CSglB9bt7RavzXsi7GDB6fT6d3Y
cwWN8D4l1PqlK+VuGd3Zay7l06BIUEigDO4Snp7KbDyE3pvjeQ3/OrtAZJLgsNZd
urVbZDgCfwUuwL+ot3ZMDgAqPfztSuVLgX8J1X4lM1Lwh6HzpRoHBH4Wsx0pzQCe
a6IJ8QtNxW3eR6KMrn1RDJFNGVV1lLfLzSXfmagT0lyoGz/m2SF7czR9E8UC54QX
NO6JAG49RolRathGyqozkDoswIg7XiRR3kPsHJAQYjFkD8A7IJIajhgG76kNGGOR
D6fUCB0iEY1n+l7Dlh7X9lStfxrV+MehVZdCfONojp1esYHM6JOufooDYg2bqqRU
xaQyZJesIzsgE3/9OGEqCNO6Oh753wNrkZDxqgNh7/OpxRkg/sjIYCAisznVTWgN
LoRlFYfAG0N03Z0UgapObpoDVro399cCAwEAATANBgkqhkiG9w0BAQsFAAOCAgEA
LQ7qKOkI9aEKPDpFoDr3rG9zCzrTAif8mJvJhpATx5HS0iHARjOjYJwug8R7pY38
yJwe31if7JZEVkEP6tS/640RNnEAncAEKZP5eWKgFQSeqGqUiFlF+8PVllQd6eQe
c1tNJ6em8ShTkfwTc1DZwTxcww9m4IGuS0BsALY15Ja6Jdd+SoVqsj9zpeLZXL8a
0WiaQsBdggb0ZfK+tLl5rdCWnaoHeBQeLfyRzzfL9+xDCRiqZQZeEv/2KrQA+AxF
l3aIz/m6PzfRSNHfwf0yXnVRrZfh+KZQ5tPhJdpzlhnv0xtXu3mT90EGYvXtLkI7
CBf4ZlWs6lZNrr4a03ZpULSWVKyZ0eDbFY8QdJZ9/rUHQOuodi1Sc3wQtXmMjzcV
nguTsUNmAAsLXkhrAB+cYf+9L9Q1HpddFZh3v7W1rrIp43APCbHP4C55nJZhKRXI
plPLd+dw0CGsmNOf28V3khFlk/Q04QRQurRF3gosLiv9J4wxAf5xsVr4H+HE5kfU
Uq1ulxcYipSknRfdehDafNk8xiUVFxGcEYnlsrfpe5ELd649HpMOLGBWJzCuLoeg
6VrLbf4atrt2IH0NXEWjb2t3FgJ/IbU6QP63F1ttOjUYKQTa25W7WPn8rweGCdtc
/C9NLi68a8T74j6Tf1ncxmIH/EaRStN8wxcun+cbnhw=
-----END CERTIFICATE-----`

const serverKey = `-----BEGIN RSA PRIVATE KEY-----
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
-----END RSA PRIVATE KEY-----
`

type testMainLoop struct {
	conns   int
	readyCh chan struct{}
	doneCh  chan struct{}
}

func (t *testMainLoop) ListenLoop(n net.Listener) error {
	t.readyCh <- struct{}{}
	for {
		c, err := n.Accept()
		if err != nil {
			break
		}
		t.conns++
		c.Write([]byte("ping"))
		c.Close()
	}
	t.doneCh <- struct{}{}
	return nil
}

func TestRunTLS(t *testing.T) {
	crt, err := writeTmp(serverCrt)
	if crt != "" {
		defer os.Remove(crt)
	}
	require.Nil(t, err, "no error")
	key, err := writeTmp(serverKey)
	if key != "" {
		defer os.Remove(key)
	}
	require.Nil(t, err, "no error")
	bindAddress := "localhost:39999"

	opts, err := ParseOptions([]string{
		"gregor",
		"--bind-address", bindAddress,
		"--auth-server", "fmprpc+tls://localhost:30000",
		"--tls-key", ("file://" + key),
		"--tls-cert", ("file://" + crt),
		"--mysql-dsn", "gregor:@/gregor_test",
	})
	require.Nil(t, err, "no error")
	require.NotNil(t, opts, "got options back")

	readyCh := make(chan struct{})
	doneCh := make(chan struct{})
	mainLoop := testMainLoop{readyCh: readyCh, doneCh: doneCh}

	srv := newMainServer(opts, &mainLoop)
	srv.stopCh = make(chan struct{})
	go func() {
		err := srv.listenAndServe()
		require.Nil(t, err, "no error")
	}()
	<-readyCh

	// Use our own test CA as above.
	certs := x509.NewCertPool()
	if !certs.AppendCertsFromPEM([]byte(caCert)) {
		t.Fatalf("unable to add ca cert")
	}
	tlsConfig := tls.Config{RootCAs: certs}

	tries := 4
	for i := 0; i < tries; i++ {
		con, err := tls.Dial("tcp", bindAddress, &tlsConfig)
		buf, err := ioutil.ReadAll(con)
		require.Nil(t, err, "no error")
		require.Equal(t, "ping", string(buf), "right payload")
	}

	srv.stopCh <- struct{}{}
	<-doneCh
	require.Equal(t, tries, mainLoop.conns, "right number of connections")
}

func TestRunTCP(t *testing.T) {
	t.Skip()
	bindAddress := "localhost:39999"

	opts, err := ParseOptions([]string{
		"gregor",
		"--bind-address", bindAddress,
		"--auth-server", "fmprpc://localhost:30000",
		"--mysql-dsn", "gregor:@/gregor_test",
	})
	require.Nil(t, err, "no error")
	require.NotNil(t, opts, "got options back")

	readyCh := make(chan struct{})
	doneCh := make(chan struct{})
	mainLoop := testMainLoop{readyCh: readyCh, doneCh: doneCh}

	srv := newMainServer(opts, &mainLoop)
	srv.stopCh = make(chan struct{})
	go func() {
		err := srv.listenAndServe()
		require.Nil(t, err, "no error")
	}()
	<-readyCh

	tries := 4
	for i := 0; i < tries; i++ {
		con, err := net.Dial("tcp", bindAddress)
		buf, err := ioutil.ReadAll(con)
		require.Nil(t, err, "no error")
		require.Equal(t, "ping", string(buf), "right payload")
	}

	srv.stopCh <- struct{}{}
	<-doneCh
	require.Equal(t, tries, mainLoop.conns, "right number of connections")
}
