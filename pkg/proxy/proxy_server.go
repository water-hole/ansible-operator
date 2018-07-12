/*
Copyright 2014 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	yaml "gopkg.in/yaml.v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	k8sproxy "k8s.io/apimachinery/pkg/util/proxy"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/transport"
	"k8s.io/kubernetes/pkg/kubectl/util"
)

const (
	// DefaultHostAcceptRE is the default value for which hosts to accept.
	DefaultHostAcceptRE = "^localhost$,^127\\.0\\.0\\.1$,^\\[::1\\]$"
	// DefaultPathAcceptRE is the default path to accept.
	DefaultPathAcceptRE = "^.*"
	// DefaultPathRejectRE is the default set of paths to reject.
	DefaultPathRejectRE = "^/api/.*/pods/.*/exec,^/api/.*/pods/.*/attach"
	// DefaultMethodRejectRE is the set of HTTP methods to reject by default.
	DefaultMethodRejectRE = "^$"
)

var (
	// ReverseProxyFlushInterval is the frequency to flush the reverse proxy.
	// Only matters for long poll connections like the one used to watch. With an
	// interval of 0 the reverse proxy will buffer content sent on any connection
	// with transfer-encoding=chunked.
	// TODO: Flush after each chunk so the client doesn't suffer a 100ms latency per
	// watch event.
	ReverseProxyFlushInterval = 100 * time.Millisecond
)

// FilterServer rejects requests which don't match one of the specified regular expressions
type FilterServer struct {
	// Only paths that match this regexp will be accepted
	AcceptPaths []*regexp.Regexp
	// Paths that match this regexp will be rejected, even if they match the above
	RejectPaths []*regexp.Regexp
	// Hosts are required to match this list of regexp
	AcceptHosts []*regexp.Regexp
	// Methods that match this regexp are rejected
	RejectMethods []*regexp.Regexp
	// The delegate to call to handle accepted requests.
	delegate http.Handler
}

// MakeRegexpArray splits a comma separated list of regexps into an array of Regexp objects.
func MakeRegexpArray(str string) ([]*regexp.Regexp, error) {
	parts := strings.Split(str, ",")
	result := make([]*regexp.Regexp, len(parts))
	for ix := range parts {
		re, err := regexp.Compile(parts[ix])
		if err != nil {
			return nil, err
		}
		result[ix] = re
	}
	return result, nil
}

// MakeRegexpArrayOrDie creates an array of regular expression objects from a string or exits.
func MakeRegexpArrayOrDie(str string) []*regexp.Regexp {
	result, err := MakeRegexpArray(str)
	if err != nil {
		log.Fatalf("Error compiling re: %v", err)
	}
	return result
}

func matchesRegexp(str string, regexps []*regexp.Regexp) bool {
	for _, re := range regexps {
		if re.MatchString(str) {
			log.Printf("%v matched %s", str, re)
			return true
		}
	}
	return false
}

func (f *FilterServer) accept(method, path, host string) bool {
	if matchesRegexp(path, f.RejectPaths) {
		return false
	}
	if matchesRegexp(method, f.RejectMethods) {
		return false
	}
	if matchesRegexp(path, f.AcceptPaths) && matchesRegexp(host, f.AcceptHosts) {
		return true
	}
	return false
}

// HandlerFor makes a shallow copy of f which passes its requests along to the
// new delegate.
func (f *FilterServer) HandlerFor(delegate http.Handler) *FilterServer {
	f2 := *f
	f2.delegate = delegate
	return &f2
}

// Get host from a host header value like "localhost" or "localhost:8080"
func extractHost(header string) (host string) {
	host, _, err := net.SplitHostPort(header)
	if err != nil {
		host = header
	}
	return host
}

func (f *FilterServer) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	host := extractHost(req.Host)
	if f.accept(req.Method, req.URL.Path, host) {
		log.Printf("Filter accepting %v %v %v", req.Method, req.URL.Path, host)
		f.delegate.ServeHTTP(rw, req)
		return
	}
	log.Printf("Filter rejecting %v %v %v", req.Method, req.URL.Path, host)
	rw.WriteHeader(http.StatusForbidden)
	rw.Write([]byte("<h3>Unauthorized</h3>"))
}

// Server is a http.Handler which proxies Kubernetes APIs to remote API server.
type Server struct {
	handler http.Handler
}

type responder struct{}

func (r *responder) Error(w http.ResponseWriter, req *http.Request, err error) {
	log.Printf("Error while proxying request: %v", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// makeUpgradeTransport creates a transport that explicitly bypasses HTTP2 support
// for proxy connections that must upgrade.
func makeUpgradeTransport(config *rest.Config) (k8sproxy.UpgradeRequestRoundTripper, error) {
	transportConfig, err := config.TransportConfig()
	if err != nil {
		return nil, err
	}
	tlsConfig, err := transport.TLSConfigFor(transportConfig)
	if err != nil {
		return nil, err
	}
	rt := utilnet.SetOldTransportDefaults(&http.Transport{
		TLSClientConfig: tlsConfig,
	})
	upgrader, err := transport.HTTPWrappersForConfig(transportConfig, k8sproxy.MirrorRequest)
	if err != nil {
		return nil, err
	}
	return k8sproxy.NewUpgradeRequestRoundTripper(rt, upgrader), nil
}

// NewServer creates and installs a new Server.
// 'filter', if non-nil, protects requests to the api only.
func NewServer(apiProxyPrefix string, filter *FilterServer, cfg *rest.Config) (*Server, error) {
	host := cfg.Host
	if !strings.HasSuffix(host, "/") {
		host = host + "/"
	}
	target, err := url.Parse(host)
	if err != nil {
		return nil, err
	}

	responder := &responder{}
	transport, err := rest.TransportFor(cfg)
	if err != nil {
		return nil, err
	}
	upgradeTransport, err := makeUpgradeTransport(cfg)
	if err != nil {
		return nil, err
	}
	proxy := k8sproxy.NewUpgradeAwareHandler(target, transport, false, false, responder)
	proxy.UpgradeTransport = upgradeTransport
	proxy.UseRequestLocation = true

	proxyServer := http.Handler(proxy)

	if !strings.HasPrefix(apiProxyPrefix, "/api") {
		proxyServer = stripLeaveSlash(apiProxyPrefix, proxyServer)
	}

	proxyServer = injectOwnerReference(proxyServer)

	mux := http.NewServeMux()
	mux.Handle(apiProxyPrefix, proxyServer)
	return &Server{handler: mux}, nil
}

// Listen is a simple wrapper around net.Listen.
func (s *Server) Listen(address string, port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("%s:%d", address, port))
}

// ListenUnix does net.Listen for a unix socket
func (s *Server) ListenUnix(path string) (net.Listener, error) {
	// Remove any socket, stale or not, but fall through for other files
	fi, err := os.Stat(path)
	if err == nil && (fi.Mode()&os.ModeSocket) != 0 {
		os.Remove(path)
	}
	// Default to only user accessible socket, caller can open up later if desired
	oldmask, _ := util.Umask(0077)
	l, err := net.Listen("unix", path)
	util.Umask(oldmask)
	return l, err
}

// ServeOnListener starts the server using given listener, loops forever.
func (s *Server) ServeOnListener(l net.Listener) error {
	server := http.Server{
		Handler: s.handler,
	}
	return server.Serve(l)
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func injectOwnerReference(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == "POST" {
			auth := map[string]string{}
			user, _, _ := req.BasicAuth()
			authString, err := base64.StdEncoding.DecodeString(user)
			if err != nil {
				panic(err)
			}
			json.Unmarshal(authString, &auth)

			reference := metav1.OwnerReference{
				APIVersion: auth["apiVersion"],
				Kind:       auth["kind"],
				Name:       auth["name"],
				UID:        types.UID(auth["uid"]),
			}
			log.Printf("%#+v", reference)

			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				panic(err)
			}
			var data unstructured.Unstructured
			err = Unmarshal(body, &data.Object)
			if err != nil {
				panic(err)
			}
			data.SetOwnerReferences(mergeOwnerReferences(reference, data.GetOwnerReferences()))
			newBody, err := json.Marshal(data.Object)
			if err != nil {
				panic(err)
			}
			log.Printf(string(newBody))
			req.Body = ioutil.NopCloser(bytes.NewBuffer(newBody))
			req.ContentLength = int64(len(newBody))
		}
		h.ServeHTTP(w, req)
	})
}

func mergeOwnerReferences(reference metav1.OwnerReference, existing []metav1.OwnerReference) []metav1.OwnerReference {
	existing = append(existing, reference)
	return existing

}

// Unmarshal YAML to map[string]interface{} instead of map[interface{}]interface{}.
func Unmarshal(in []byte, out interface{}) error {
	var res map[string]interface{}

	if err := yaml.Unmarshal(in, &res); err != nil {
		return err
	}
	*out.(*map[string]interface{}) = cleanupMapValue(res).(map[string]interface{})

	return nil
}

// Marshal YAML wrapper function.
func Marshal(in interface{}) ([]byte, error) {
	return yaml.Marshal(in)
}

func cleanupInterfaceArray(in []interface{}) []interface{} {
	res := make([]interface{}, len(in))
	for i, v := range in {
		res[i] = cleanupMapValue(v)
	}
	return res
}

func cleanupInterfaceMap(in map[interface{}]interface{}) map[string]interface{} {
	res := make(map[string]interface{})
	for k, v := range in {
		res[fmt.Sprintf("%v", k)] = cleanupMapValue(v)
	}
	return res
}

func cleanupMapValue(v interface{}) interface{} {
	switch v := v.(type) {
	case map[string]interface{}:
		for key, val := range v {
			v[key] = cleanupMapValue(val)
		}
		return v
	case []interface{}:
		return cleanupInterfaceArray(v)
	case map[interface{}]interface{}:
		return cleanupInterfaceMap(v)
	default:
		return v
	}
}

// like http.StripPrefix, but always leaves an initial slash. (so that our
// regexps will work.)
func stripLeaveSlash(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p := strings.TrimPrefix(req.URL.Path, prefix)
		if len(p) >= len(req.URL.Path) {
			http.NotFound(w, req)
			return
		}
		if len(p) > 0 && p[:1] != "/" {
			p = "/" + p
		}
		req.URL.Path = p
		h.ServeHTTP(w, req)
	})
}
