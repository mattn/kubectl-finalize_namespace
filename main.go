package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"
)

const name = "kubectl-finalize_namespace"

const version = "0.0.2"

var revision = "HEAD"

func fatalIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func main() {
	var showVersion bool
	flag.BoolVar(&showVersion, "V", false, "Print the version")
	flag.Parse()

	if showVersion {
		fmt.Printf("%s %s (rev: %s/%s)\n", name, version, revision, runtime.Version())
		return
	}

	namespace := flag.Arg(0)
	if namespace == "" {
		flag.Usage()
		os.Exit(1)
	}

	cmd := exec.Command("kubectl", "get", "namespace", namespace, "-o", "json")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	err := cmd.Run()
	fatalIf(err)
	var v interface{}
	err = json.Unmarshal(buf.Bytes(), &v)
	fatalIf(err)

	m := v
	if vv, ok := m.(map[string]interface{}); ok {
		m = vv["status"]
		if vv, ok := m.(map[string]interface{}); ok {
			m = vv["phase"]
		} else {
			fatalIf(errors.New("invalid json"))
		}
		if vv, ok := m.(string); ok {
			if vv != "Terminating" {
				return
			}
		} else {
			fatalIf(errors.New("invalid json"))
		}
	} else {
		fatalIf(errors.New("invalid json"))
	}

	m = v
	if vv, ok := m.(map[string]interface{}); ok {
		m = vv["spec"]
		if vv, ok := m.(map[string]interface{}); ok {
			vv["finalizers"] = []string{}
		} else {
			fatalIf(errors.New("invalid json"))
		}
	} else {
		fatalIf(errors.New("invalid json"))
	}

	buf.Reset()
	err = json.NewEncoder(&buf).Encode(v)
	fatalIf(err)

	req, err := http.NewRequest(http.MethodPut, "http://127.0.0.1:8001/api/v1/namespaces/"+namespace+"/finalize", &buf)
	fatalIf(err)

	cmd = exec.Command("kubectl", "proxy")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		cmd.Run()
	}()

	time.Sleep(1 * time.Second)

	req.Header.Add("content-type", "application/json")
	resp, err := http.DefaultClient.Do(req)

	cmd.Process.Kill()
	wg.Wait()

	if err != nil {
		fatalIf(err)
	}

	defer resp.Body.Close()
	io.Copy(os.Stdout, resp.Body)
}
