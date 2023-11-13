package metrics_scraper

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
)

//#region fakeHttpClient

type fakeReader struct {
	io.Reader
	IsClosed bool
}

func newFakeReader(content string) *fakeReader {
	return &fakeReader{Reader: strings.NewReader(content)}
}

func newFakeReaderBinary(content []byte) *fakeReader {
	return &fakeReader{Reader: bytes.NewReader(content)}
}

func (fr *fakeReader) Close() error {
	fr.IsClosed = true
	return nil
}

type fakeHttpClient struct {
	Request           *http.Request
	Response          *http.Response
	Err               error
	ResposeBodyReader *fakeReader
}

func newFakeHttpClient(responseBody interface{}) *fakeHttpClient {
	switch responseBodyTyped := responseBody.(type) {
	case string:
		reader := newFakeReader(responseBodyTyped)
		return &fakeHttpClient{
			Response: &http.Response{
				StatusCode: 200,
				Body:       reader,
			},
			ResposeBodyReader: reader,
		}
	case []byte:
		reader := newFakeReaderBinary(responseBodyTyped)
		return &fakeHttpClient{
			Response: &http.Response{
				StatusCode: 200,
				Body:       reader,
			},
			ResposeBodyReader: reader,
		}
	default:
		panic("creating FakeHTTPClient: the input type is not supported")
	}
}

func (fc *fakeHttpClient) Do(request *http.Request) (*http.Response, error) {
	fc.Request = request
	if fc.Err != nil {
		return nil, fc.Err
	}
	return fc.Response, nil
}

//#endregion fakeHttpClient

var _ = Describe("input.metrics_scraper.metricsClientImpl", func() {
	const (
		metricsUrl = "https://my/metrics"
		authSecret = "auth secret"
	)

	var (
		certPool = getExampleCertPool()
	)
	var (
		newTestMetricsClient = func(responseBody interface{}) (*metricsClientImpl, *fakeHttpClient) {
			metricsClient := newMetricsClient().(*metricsClientImpl)
			httpClient := newFakeHttpClient(responseBody)
			metricsClient.testIsolation.NewHttpClient = func(caCertificates *x509.CertPool) rest.HTTPClient {
				return httpClient
			}
			return metricsClient, httpClient
		}
		newResponseBody = func(extraContent string) string {
			return `# HELP something something` + "\n" +
				`some_metric{code="200",component="apiserver"} 15` + "\n" +
				`# HELP something something` + "\n" +
				`# HELP something something` + "\n" +
				`some_metric{code="200"} 15` + "\n" +
				extraContent +
				`some_metric{code="201",component="apiserver"} 15` + "\n" +
				`some_metric{code="201"} 15` + "\n" +
				`another_metric{code="200",component="apiserver"} 15` + "\n" +
				`another_metric{code="200"} 15` + "\n" +
				`another_metric{code="201",component="apiserver"} 15` + "\n" +
				`another_metric{code="201"} 15` + "\n" +
				`another_metric{code="202",component="apiserver"} 15` + "\n" +
				`another_metric{code="202"} 15`
		}
	)

	Describe("metricsClientImpl.GetKapiInstanceMetrics", func() {
		It("should return an error and zero value when the HTTP request call returns an error", func() {
			// Arrange
			mc, http := newTestMetricsClient("")
			http.Err = errors.New("my error")

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(http.Err.Error()))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when the HTTP call returns HTTP error code", func() {
			// Arrange
			mc, http := newTestMetricsClient("")
			http.Response.StatusCode = 400

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(ContainSubstring(fmt.Sprint(http.Response.StatusCode)))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when the HTTP response is empty", func() {
			// Arrange
			mc, _ := newTestMetricsClient("")

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*no.*counters.*"))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when the HTTP response payload is binary and not text data", func() {
			// Arrange
			mc, _ := newTestMetricsClient([]byte{1, 5, 10, 20, 40, 80, 160})

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when the HTTP response is missing the RPS metric", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(""))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*no.*counters.*"))
			Expect(result).To(BeZero())
		})
		It("should succeed when an RPS metric line has a positive int32 value", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} 5678\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(5678)))
		})
		It("should sum up all RPS metric counters", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody(
				"apiserver_request_total{code=\"200\"} 15\n" +
					"other_metric 50\n" +
					"apiserver_request_total{code=\"201\"} 16\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(31)))
		})
		It("should succeed when an RPS metric line has a negative int64 value which does not fit in int32", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} -10000000000\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(-10 * 1000 * 1000 * 1000)))
		})
		It("should succeed when an RPS metric line has a floating point value which corresponds to an integer", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} 1.0056e4\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(10056)))
		})
		It("should succeed when an RPS metric line has no series identifier", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total 15\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should succeed if an RPS metric line has whitespace between the metric name and the series identifier", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total \t{code=\"200\"} 15\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should return an error and zero value when an RPS metric line has unterminated series identifier", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\" 15\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*malformed.*"))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when an RPS metric line is missing the value", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"}\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*malformed.*"))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when an RPS metric line has a value which is not a number", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} BadValue\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*malformed.*"))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when an RPS metric line has a floating point value which does not correspond to an integer", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} 1.5\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*malformed.*"))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when an RPS metric line has an integer value which does not fit in int64", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} 99999999999999999999\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*malformed.*"))
			Expect(result).To(BeZero())
		})
		It("should return an error and zero value when an RPS metric line contains a zero byte", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total\x00{code=\"200\"} 15\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).NotTo(BeNil())
			Expect(err.Error()).To(MatchRegexp(".*malformed.*"))
			Expect(result).To(BeZero())
		})
		It("should ignore empty lines", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody(newResponseBody("\n\napiserver_request_total{code=\"200\"} 15\n")))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should attempt to parse the response as plaintext metrics, when the HTTP response has unexpected content encoding", func() {
			// Arrange
			mc, http := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} 15\n")))
			http.Response.Header = map[string][]string{"Content-Encoding": {"surprise"}}

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should succeed when the HTTP response payload starts with a comment", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody("# HELP abc\napiserver_request_total{code=\"200\"} 15\n"))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should succeed when the HTTP response payload does not start with a comment", func() {
			// Arrange
			mc, _ := newTestMetricsClient(newResponseBody("apiserver_request_total{code=\"200\"} 15\n"))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should succeed when the HTTP response is gzip compressed", func() {
			// Arrange
			gzipBytes, err := os.ReadFile("testdata/metrics-response-sample.gz")
			Expect(err).To(Succeed())
			mc, http := newTestMetricsClient(gzipBytes)
			http.Response.Header = map[string][]string{"Content-Encoding": {"gzip"}}

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(15)))
		})
		It("should process correctly a 100MB plain text HTTP response", func() {
			// Arrange
			var commentBuilder strings.Builder
			commentBuilder.Grow(100 * 1000)
			for i := 0; i < 99999; i++ {
				commentBuilder.WriteByte('#')
			}
			commentBuilder.WriteByte('\n')
			comment := commentBuilder.String()

			var responseBuilder strings.Builder
			for i := 0; i < 1000; i++ {
				responseBuilder.WriteString(comment)
			}

			counterCount := 10 * 1000
			for i := 0; i < counterCount; i++ {
				responseBuilder.WriteString("apiserver_request_total{code=\"200\"} 2\n")
			}
			mc, _ := newTestMetricsClient(newResponseBody(responseBuilder.String()))

			// Act
			result, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)

			// Assert
			Expect(err).To(BeNil())
			Expect(result).To(Equal(int64(2 * counterCount)))
		})
		It("when failing, should close the response stream", func() {
			// Arrange
			mc, http := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\" 15\n")))

			// Act
			_, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)
			Expect(err).NotTo(BeNil())

			// Assert
			Expect(http.ResposeBodyReader.IsClosed).To(BeTrue())
		})
		It("when succeeding, should close the response stream", func() {
			// Arrange
			mc, http := newTestMetricsClient(newResponseBody(newResponseBody("apiserver_request_total{code=\"200\"} 15\n")))

			// Act
			_, err := mc.GetKapiInstanceMetrics(context.Background(), metricsUrl, authSecret, certPool)
			Expect(err).To(BeNil())

			// Assert
			Expect(http.ResposeBodyReader.IsClosed).To(BeTrue())
		})
		It("should pass the correct parameters to the HTTP requests it makes", func() {
			// Arrange
			mc, http := newTestMetricsClient("")

			// Act
			mc.GetKapiInstanceMetrics(context.Background(), "https://my/metrics", authSecret, certPool)

			// Assert
			Expect(http.Request.URL.Scheme).To(Equal("https"))
			Expect(http.Request.URL.Host).To(Equal("my"))
			Expect(http.Request.URL.Path).To(Equal("/metrics"))
			Expect(http.Request.Header["Authorization"]).To(Equal([]string{"Bearer " + authSecret}))
		})
		It("should pass the specified context to the HTTP client, so it can abort work when context is cancelled", func() {
			// Arrange
			mc, http := newTestMetricsClient("")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Act
			mc.GetKapiInstanceMetrics(ctx, "https://my/metrics", authSecret, certPool)

			// Assert
			Expect(http.Request.Context().Err()).To(BeNil())
			cancel()
			Expect(http.Request.Context().Err()).ToNot(BeNil())
		})
	})
	Describe("newMetricsClient", func() {
		It("should return a client which uses specified cert pool for HTTP clients it creates", func() {
			// Arrange
			mc := newMetricsClient().(*metricsClientImpl)

			// Act
			hc := mc.testIsolation.NewHttpClient(certPool)

			// Assert
			actualCertPool := hc.(*http.Client).Transport.(*http.Transport).TLSClientConfig.RootCAs
			Expect(actualCertPool == certPool).To(BeTrue())
		})
	})
})
