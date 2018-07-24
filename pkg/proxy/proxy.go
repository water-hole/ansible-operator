package proxy

// This file contains this project's custom code, as opposed to kubectl.go
// which contains code retrieved from the kubernetes project.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strings"

	"github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

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
		if req.Method == http.MethodPost {
			logrus.Info("injecting owner reference")
			dump, _ := httputil.DumpRequest(req, false)
			fmt.Println(string(dump))

			user, _, ok := req.BasicAuth()
			if !ok {
				logrus.Error("basic auth header not found")
				w.Header().Set("WWW-Authenticate", "Basic realm=\"Operator Proxy\"")
				http.Error(w, "", http.StatusUnauthorized)
				return
			}
			authString, err := base64.StdEncoding.DecodeString(user)
			if err != nil {
				m := "could not base64 decode username"
				logrus.Errorf("%s: %s", err.Error())
				http.Error(w, m, http.StatusBadRequest)
				return
			}
			owner := metav1.OwnerReference{}
			json.Unmarshal(authString, &owner)

			logrus.Printf("%#+v", owner)

			body, err := ioutil.ReadAll(req.Body)
			if err != nil {
				m := "could not read request body"
				logrus.Errorf("%s: %s", err.Error())
				http.Error(w, m, http.StatusInternalServerError)
				return
			}
			var data unstructured.Unstructured
			err = unmarshal(body, &data.Object)
			if err != nil {
				m := "could not deserialize request body"
				logrus.Errorf("%s: %s", err.Error())
				http.Error(w, m, http.StatusBadRequest)
				return
			}
			data.SetOwnerReferences(append(data.GetOwnerReferences(), owner))
			newBody, err := json.Marshal(data.Object)
			if err != nil {
				m := "could not serialize body"
				logrus.Errorf("%s: %s", err.Error())
				http.Error(w, m, http.StatusInternalServerError)
				return
			}
			logrus.Printf(string(newBody))
			req.Body = ioutil.NopCloser(bytes.NewBuffer(newBody))
			req.ContentLength = int64(len(newBody))
		}
		// Removing the authorization so that the proxy can set the correct authorization.
		req.Header.Del("Authorization")
		h.ServeHTTP(w, req)
	})
}

// unmarshal YAML to map[string]interface{} instead of map[interface{}]interface{}.
func unmarshal(in []byte, out interface{}) error {
	var res map[string]interface{}

	if err := yaml.Unmarshal(in, &res); err != nil {
		return err
	}
	*out.(*map[string]interface{}) = cleanupMapValue(res).(map[string]interface{})

	return nil
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
