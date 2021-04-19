package testhttpapi

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"sync"
	"testing"
	"os"
	"encoding/json"
	"strings"
	"reflect"

	"infraql/internal/test/testutil"
)

type HTTPRequestExpectations struct {
	Body   io.ReadCloser
	Header http.Header
	Method string
	URL    *url.URL
	Host string
	ResponseExpectations HTTPResponseExpectations
}

type HTTPResponseExpectations struct {
	Body string
	Header http.Header
}

type ExpectationList struct {
	mu sync.Mutex
	Pos int
	Ex []HTTPRequestExpectations
}

type ExpectationStore map[string]*ExpectationList

func NewExpectationStore() ExpectationStore {
	return make(ExpectationStore)
}

func (ex ExpectationStore) Put(k string, v HTTPRequestExpectations) {
	eL, ok := ex[k]
	if ok {
		eL.Ex = append(eL.Ex, v)
		ex[k] = eL
		return
	}
	ex[k] = &ExpectationList{Pos: 0, Ex: []HTTPRequestExpectations{v}}
}

func (ex ExpectationStore) Get(k string) (HTTPRequestExpectations, bool) {
	eL, ok := ex[k]
	if ok {
		eL.mu.Lock()
		defer eL.mu.Unlock()
		if eL.Pos < len(eL.Ex) {
			rv := eL.Ex[eL.Pos]
			eL.Pos++
			return rv, true
		}
	}
	return HTTPRequestExpectations{}, false
}

func (ex ExpectationStore) HasKey(k string) bool {
	eL, ok := ex[k]
	if ok {
		eL.mu.Lock()
		defer eL.mu.Unlock()
		if eL.Pos < len(eL.Ex) {
			return true
		}
	}
	return false
}

func (ex ExpectationStore) Keys() []string {
	var rv []string
	for k := range ex {
		rv = append(rv, k)
	}
	return rv
}


type SimulatedRoundTripper struct {
	T            *testing.T
	Expectations ExpectationStore
	RoundTripper func(*http.Request) (*http.Response, error)
	Strict       bool
}

func newSimpleTransportHandler(ex ExpectationStore) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		expectations, ok := ex.Get(req.Host + req.URL.Path)
		err := compareHTTPRequestToExpected(req, &expectations)
		if err != nil {
			return nil, err
		}
		responseHeader := make(http.Header)
		var responseBody io.ReadCloser
		if ok {
			responseHeader = expectations.ResponseExpectations.Header
			responseBody = testutil.CreateReadCloserFromString(expectations.ResponseExpectations.Body)
		}
		response := &http.Response{
			Proto:      "HTTP/1.1",
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     responseHeader,
			Body:       responseBody,
			Request:    req,
		} 
		return response, nil
	}
}

func (srt SimulatedRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	reqKey := req.Host + req.URL.Path
	ok := srt.Expectations.HasKey(reqKey)
	srt.T.Logf("searching for expectations with key = '%s', found = %v", reqKey, ok)
	if !ok && srt.Strict {
		srt.T.Fatalf("FAIL: no expectations found for key '%s' in strict mode, existing keys  = %s", reqKey, strings.Join(srt.Expectations.Keys(), ", "))
	}
	return srt.RoundTripper(req)
}

func NewURL(scheme, host, path string) *url.URL {
	return &url.URL{
		Scheme: scheme,
		Host: host,
		Path: path,
	}
}

func NewSimulatedRoundTripper(t *testing.T, expectations ExpectationStore, roundTripper func(*http.Request) (*http.Response, error), strict bool) SimulatedRoundTripper {
	return SimulatedRoundTripper{
		T:            t,
		Expectations: expectations,
		RoundTripper: roundTripper,
		Strict:       strict,
	}
}

func NewHTTPRequestExpectations(body io.ReadCloser, header http.Header, method string, url *url.URL, host string, responseBody string, reponseHeader http.Header) *HTTPRequestExpectations {
	return &HTTPRequestExpectations{
		Body:   body,
		Header: header,
		Method: method,
		URL:    url,
		Host: host,
		ResponseExpectations: HTTPResponseExpectations{
			Body: responseBody,
			Header: reponseHeader,
		},
	}
}

func DefaultHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello %s\n", path.Base(r.URL.Path))
}

func getBodyMap(bodyBytes []byte, contentTypeHeader []string) (map[string]interface{}, error) {
	retVal :=  make(map[string]interface{})
	var err error
	for _, contentType := range contentTypeHeader {
		switch contentType {
			case "application/json":
				err = json.Unmarshal(bodyBytes, &retVal)
				return retVal, err
		}
	}
	return nil, fmt.Errorf("could not find acceptable content type in content type header: %s", strings.Join(contentTypeHeader, ", "))
}

func compareHTTPBodyToExpected(req *http.Request, expectations *HTTPRequestExpectations) (io.ReadCloser, error) {
	var actualBodyBytes, expectedBodyBytes []byte
	var err error
	var retVal io.ReadCloser
	if expectations.Body != nil {
		actualBodyBytes, err = ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading actual body")
		}
		expectedBodyBytes, err = ioutil.ReadAll(expectations.Body)
		if err != nil {
			return nil, fmt.Errorf("error reading expected body")
		}
		actualBodyMap, err := getBodyMap(actualBodyBytes, req.Header["Content-Type"])
		if err != nil {
			return nil, fmt.Errorf("error parsing actual body")
		}
		expectedBodyMap, err := getBodyMap(expectedBodyBytes, req.Header["Content-Type"])
		if err != nil {
			return nil, fmt.Errorf("error parsing expected body")
		}
		 
		if !reflect.DeepEqual(actualBodyMap, expectedBodyMap) {
			return nil, fmt.Errorf("http request body: actual != expected: '%s' != '%s'", string(actualBodyBytes), string(expectedBodyBytes))
		}
		expectations.Body = ioutil.NopCloser(bytes.NewReader(expectedBodyBytes))
		retVal = ioutil.NopCloser(bytes.NewReader(actualBodyBytes))
		fmt.Fprintln(os.Stderr, "body check completed successfully!")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, fmt.Sprintf("body = '%s'", string(actualBodyBytes)))
		fmt.Fprintln(os.Stderr, "")
	}
	return retVal, nil
}

func compareHTTPHeaderToExpected(actualHeader http.Header, expectations *HTTPRequestExpectations) error {
	if expectations.Header == nil {
		return nil
	}
	for k, v := range expectations.Header {
		av, ok := actualHeader[k]
		if !ok {
			return fmt.Errorf("missing expected header key '%s'", k)
		}
		actualVals := make(map[string]bool)
		for i := range av {
			actualVals[av[i]] = true
		}
		for i := range v {
			if !actualVals[v[i]] {
				return fmt.Errorf("missing expected header value '%s' for k '%s'", v[i], k)
			}
			fmt.Fprintln(os.Stderr, fmt.Sprintf("header key '%s' and val '%s' OK", k, v[i]))
		}
	}
	return nil
}

func compareHTTPURLToExpected(actualURL *url.URL, expectations *HTTPRequestExpectations) error {
	if expectations.URL == nil {
		return nil
	}
	var err error
	err = compareExpectedStrings(actualURL.Scheme, expectations.URL.Scheme, "Scheme")
	if err != nil {
		return err
	}
	err = compareExpectedStrings(actualURL.Host, expectations.URL.Host, "Host")
	if err != nil {
		return err
	}
	err = compareExpectedStrings(actualURL.Path, expectations.URL.Path, "Path")
	if err != nil {
		return err
	}
	return compareExpectedStringsStrict(actualURL.RawQuery, expectations.URL.RawQuery, "RawQuery")
}

func compareExpectedStrings(actual string, expected string, descriptor string) error {
	if expected == "" {
		// fmt.Fprintln(os.Stderr, "skipping comparing %s strings; expected : actual: '%s' : '%s'", descriptor, expected, actual)
		return nil
	}
	return compareExpectedStringsStrict(actual, expected, descriptor)
}

func compareExpectedStringsStrict(actual string, expected string, descriptor string) error {
	if expected != actual {
		return fmt.Errorf("error comparing %s strings; expected != actual: '%s' != '%s'", descriptor, expected, actual)
	}
	// fmt.Fprintln(os.Stderr, "success comparing %s strings; expected == actual: '%s' == '%s'", descriptor, expected, actual)
	return nil
}

func compareHTTPRequestToExpected(req *http.Request, expectations *HTTPRequestExpectations) error {
	var err error
	if expectations != nil {
		if expectations.Body != nil {
			req.Body, err = compareHTTPBodyToExpected(req, expectations)
			if err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "SUCCESS: request body checks out ok!")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "")
		}
		if expectations.Method != "" {
			if req.Method != expectations.Method {
				return fmt.Errorf("FAIL: http request method: actual != expected: '%s' != '%s'", req.Method, expectations.Method)
			}
			fmt.Fprintln(os.Stderr, "SUCCESS: request method checks out ok!")
		}
		err = compareHTTPURLToExpected(req.URL, expectations)
		if err != nil {
			return err
		}
		return compareHTTPHeaderToExpected(req.Header, expectations)
	}
	return err
}

func GetRequestTestHandler(t *testing.T, expectationStore ExpectationStore, handler http.HandlerFunc) http.HandlerFunc {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			reqKey := r.Host + r.URL.Path
			expectations, ok := expectationStore.Get(reqKey)
			t.Logf("searching for expectations with key = '%s', found = %v", reqKey, ok)
			if ok {
				err := compareHTTPRequestToExpected(r, &expectations)
				if err != nil {
					t.Fatalf("Test failed: %s", err.Error())
				}
				if expectations.ResponseExpectations.Body == "" {
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				fmt.Fprintf(w, expectations.ResponseExpectations.Body)
				return
			}
			if handler == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			handler(w, r)
		},
	)
}

func SetupHTTPCallHeavyweight(t *testing.T, expectationStore ExpectationStore, handlerFunc http.HandlerFunc, roundTripper http.RoundTripper) {
	handler := GetRequestTestHandler(t, expectationStore, handlerFunc)
	s := httptest.NewServer(http.HandlerFunc(handler))
	u, err := url.Parse(s.URL)
	if err != nil {
		t.Fatalf("FAIL: failed to parse httptest.Server URL: %v", err)
	}
	http.DefaultClient.Transport = NewRewriteTransport(roundTripper, u)
}

// RewriteTransport is an http.RoundTripper that rewrites requests
// using the provided URL's Scheme and Host, and its Path as a prefix.
// The Opaque field is untouched.
// If Transport is nil, http.DefaultTransport is used
type RewriteTransport struct {
	Transport http.RoundTripper
	URL       *url.URL
}

func NewRewriteTransport(transport http.RoundTripper, url *url.URL) RewriteTransport {
	return RewriteTransport{
		Transport: transport,
		URL:       url,
	}
}

func (t RewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// note that url.URL.ResolveReference doesn't work here
	// since t.u is an absolute url
	req.URL.Scheme = t.URL.Scheme
	req.URL.Host = t.URL.Host
	req.URL.Path = path.Join(t.URL.Path, req.URL.Path)
	rt := t.Transport
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(req)
}

type HandlerTransport struct {
	Handler             http.Handler
	ResponseTransformer func(response *http.Response) *http.Response
}

func NewHandlerTransport(handler http.Handler, responseTransformer func(response *http.Response) *http.Response) HandlerTransport {
	return HandlerTransport{
		Handler:             handler,
		ResponseTransformer: responseTransformer,
	}
}

func (t HandlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r, w := io.Pipe()
	resp := &http.Response{
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       r,
		Request:    req,
	}
	ready := make(chan struct{})
	prw := &PipeResponseWriter{r, w, resp, ready}
	go func() {
		defer w.Close()
		t.Handler.ServeHTTP(prw, req)
	}()
	<-ready
	if t.ResponseTransformer != nil {
		resp = t.ResponseTransformer(resp)
	}
	return resp, nil
}

type PipeResponseWriter struct {
	r     *io.PipeReader
	w     *io.PipeWriter
	resp  *http.Response
	ready chan<- struct{}
}

func (w *PipeResponseWriter) Header() http.Header {
	return w.resp.Header
}

func (w *PipeResponseWriter) Write(p []byte) (int, error) {
	if w.ready != nil {
		w.WriteHeader(http.StatusOK)
	}
	return w.w.Write(p)
}

func (w *PipeResponseWriter) WriteHeader(status int) {
	if w.ready == nil {
		// already called
		return
	}
	w.resp.StatusCode = status
	w.resp.Status = fmt.Sprintf("%d %s", status, http.StatusText(status))
	close(w.ready)
	w.ready = nil
}

func ValidateHTTPResponseAndErr(t *testing.T, response *http.Response, err error) {
	if err == nil {
		if response.Body != nil {
			bb, bErr := ioutil.ReadAll(response.Body)
			if bErr != nil {
				t.Fatalf("could not read body: %v", bErr)
			}
			t.Logf("response body = '%s'", string(bb))
			return
		}
		t.Logf("response = %v", response)
		return
	}
	t.Fatalf("HTTPS call failed: %v", err)
}

func StartServer(t *testing.T, expectations ExpectationStore) {
	transport := newSimpleTransportHandler(expectations)
	var roundTripper http.RoundTripper = NewSimulatedRoundTripper(t, expectations, transport, true)
	SetupHTTPCallHeavyweight(t, expectations, DefaultHandler, roundTripper)
}
