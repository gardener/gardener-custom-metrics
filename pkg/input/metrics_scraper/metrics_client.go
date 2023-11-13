package metrics_scraper

import (
	"bufio"
	"compress/gzip"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	krest "k8s.io/client-go/rest"
)

const (
	metricName = "apiserver_request_total"
)

type metricsClient interface {
	// GetKapiInstanceMetrics scrapes a Kapi metric endpoint and returns the sum of all apiserver_request_total counters.
	//
	// Parameters:
	//   - url points to the metrics endpoint.
	//   - authSecret specifies a bearer auth token to present to the metrics endpoint.
	//   - caCertificates lists trusted CA certificates which are used to verify the endpoint's certificate.
	//
	// Returns:
	//   - an int64 value which is the sum of all apiserver_request_total counters from the scraped metric response.
	//   - an optional error
	//
	// Exactly one of the int64 value and the error is non-zero.
	// An error is returned if the metrics data contains no apiserver_request_total counters.
	//
	// Remarks: For performance reasons, this function requires that if a line containing the metric of interest start with
	// whitespaces, those whitespaces be only ASCII whitespaces.
	GetKapiInstanceMetrics(
		ctx context.Context, url string, authSecret string, caCertificates *x509.CertPool) (result int64, err error)
}

type metricsClientImpl struct {
	testIsolation metricsClientTestIsolation // Provides indirections necessary to isolate the unit during tests
}

func newMetricsClient() metricsClient {
	return &metricsClientImpl{
		testIsolation: metricsClientTestIsolation{
			NewHttpClient: newHttpClient,
		},
	}
}

// GetKapiInstanceMetrics scrapes a Kapi metric endpoint and returns the sum of all apiserver_request_total counters.
//
// Parameters:
//   - url points to the metrics endpoint.
//   - authSecret specifies a bearer auth token to present to the metrics endpoint.
//   - caCertificates lists trusted CA certificates which are used to verify the endpoint's certificate.
//
// Returns:
//   - an int64 value which is the sum of all apiserver_request_total counters from the scraped metric response.
//   - an optional error
//
// Exactly one of the int64 value and the error is non-zero.
// An error is returned if the metrics data contains no apiserver_request_total counters.
//
// Remarks: For performance reasons, this function requires that if a line containing the metric of interest start with
// whitespaces, those whitespaces be only ASCII whitespaces.
func (mc *metricsClientImpl) GetKapiInstanceMetrics(
	ctx context.Context, url string, authSecret string, caCertificates *x509.CertPool) (result int64, err error) {

	// Prepare request
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("metrics client: creating http request object: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+authSecret)
	request.Header.Set("Accept-Encoding", "gzip")
	client := mc.testIsolation.NewHttpClient(caCertificates)

	// Send request
	response, err := client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("metrics client: making http request: %w", err)
	}
	defer func(responseBodyStream io.ReadCloser) {
		e := responseBodyStream.Close()
		if e != nil && err == nil {
			err = fmt.Errorf("metrics client: closing response stream: %w", e)
		}
	}(response.Body)

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("metrics client: responce reported HTTP status %d", response.StatusCode)
	}

	// If the server returned compressed response, use decompressing reader
	if response.Header.Get("Content-Encoding") == "gzip" {
		reader, err := gzip.NewReader(response.Body)
		if err != nil {
			return 0, fmt.Errorf("metrics client: scraping '%s': reading gzip encoded response stream: %w", url, err)
		}
		defer reader.Close()

		return getTotalRequestCount(reader)
	}

	return getTotalRequestCount(response.Body)
}

// getTotalRequestCount processes a metrics response stream and returns the sum of all apiserver_request_total counters.
//
// Returns:
//   - an int64 value which is the sum of all apiserver_request_total counters from the scraped metric response.
//   - an optional error
//
// Exactly one of the int64 value and the error is non-zero.
func getTotalRequestCount(metricsStream io.ReadCloser) (int64, error) {
	reader := bufio.NewReader(metricsStream)

	totalRequestCount := int64(0)
	isCounterFound := false
	isLastReadPartial := false
	lineBytes, isPrefix, err := reader.ReadLine()
	for ; err == nil; lineBytes, isPrefix, err = reader.ReadLine() {
		if isPrefix {
			// Long lines are not expected, and not of interest to us. Just skip them.
			isLastReadPartial = true
			continue
		}

		if isLastReadPartial {
			// That's the last fragment of a long line
			isLastReadPartial = false
			continue
		}

		line := string(lineBytes)
		if len(line) > 0 && isSpace(line, 0) {
			i := skipSpace(line, 1)
			line = line[i:]
		}
		if !strings.HasPrefix(line, metricName) {
			// One of the other metrics. Not of interest to us.
			continue
		}

		_, seriesCurrentValue, err := parseLine(line)
		if err != nil {
			return 0, fmt.Errorf("parsing metrics line '%s': %w", line, err)
		}

		totalRequestCount += seriesCurrentValue
		isCounterFound = true
	}

	if err != io.EOF {
		return 0, err
	}

	if !isCounterFound {
		return 0, fmt.Errorf(
			"calculating total request count from metrics response: the response contains no '%s' counters", metricName)
	}

	return totalRequestCount, nil
}

// Assumes that the line starts with metricName, no leading whitespace.
// Returns (seriesId, seriesValue, error). Exactly one of seriesValue/error is nil.
func parseLine(line string) (string, int64, error) {
	// Sample line: apiserver_request_total{code="200",component="apiserver",dry_run="",group="",resource="configmaps",scope="namespace",subresource="",verb="LIST",version="v1"} 15

	malformedLineError := fmt.Errorf("parsing metrics line: malformed line '%s'", line)
	seriesId := ""

	// Process series name section, e.g: {code="200",component="apiserver",dry_run="",group="",resource="configmaps",scope="namespace",subresource="",verb="LIST",version="v1"}
	i := len(metricName)
	if i >= len(line) {
		return "", 0, malformedLineError
	}

	// Process optional labels section
	i = skipSpace(line, i)
	if line[i] == '{' {
		seriesIdStart := i + 1

		for i++; i < len(line) && line[i] != '}'; i++ {
		}
		if i == len(line) {
			return "", 0, malformedLineError
		}

		seriesId = line[seriesIdStart:i]
		i++ // Move past '}'
	}

	// Process value section
	i = skipSpace(line, i)
	if i >= len(line) {
		return "", 0, malformedLineError
	}
	valueEnd := i + 1
	for ; valueEnd < len(line) && !isSpace(line, valueEnd); valueEnd++ {
	}
	valueString := line[i:valueEnd]
	var seriesValue int64
	var err error
	if strings.Contains(valueString, "e") { // Some integer values come in scientific notation, e.g. 1.234567e+06
		var floatValue float64
		floatValue, err = strconv.ParseFloat(valueString, 64)
		seriesValue = int64(floatValue) // The significand of double is 53 bits - should represent request count accurately
	} else {
		seriesValue, err = strconv.ParseInt(valueString, 10, 64)
	}
	if err != nil {
		return "", 0, malformedLineError
	}

	return seriesId, seriesValue, nil
}

func isSpace(str string, i int) bool {
	return str[i] == ' ' || str[i] == '\t'
}

// Starts at i and returns the index of the first non whitespace character, or one-past-end
func skipSpace(str string, i int) int {
	for ; i < len(str) && isSpace(str, i); i++ {
	}
	return i
}

//#region Test isolation

// metricsClientTestIsolation contains all points of indirection necessary to isolate static function calls
// in the metrics client unit
type metricsClientTestIsolation struct {
	// Creates a new HTTP client with default settings
	NewHttpClient func(caCertificates *x509.CertPool) krest.HTTPClient
}

func newHttpClient(caCertificates *x509.CertPool) krest.HTTPClient {
	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caCertificates,
				ServerName: "kube-apiserver",
				MinVersion: tls.VersionTLS13,
			},
		},
	}
}

//#endregion Test isolation
