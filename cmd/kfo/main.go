/*
Copyright 2026.

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

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	freezev1alpha1 "github.com/jamalshahverdiev/kube-freeze-operator/api/v1alpha1"
	"github.com/jamalshahverdiev/kube-freeze-operator/internal/policy"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(freezev1alpha1.AddToScheme(scheme))
}

type apiResponse struct {
	Allow         bool    `json:"allow"`
	Reason        string  `json:"reason,omitempty"`
	MatchedPolicy string  `json:"matchedPolicy,omitempty"`
	PolicyKind    string  `json:"policyKind,omitempty"`
	FreezeEndTime *string `json:"freezeEndTime,omitempty"`
	NextAllowed   *string `json:"nextAllowedTime,omitempty"`
	EvaluatedAt   string  `json:"evaluatedAt"`
	Error         string  `json:"error,omitempty"`
}

func usage() {
	fmt.Fprintf(os.Stderr, `kfo — kube-freeze-operator CLI

Usage:
  kfo can-i --namespace <ns> --kind <kind> --action <action> [flags]

Flags:
  --namespace, -n    Target namespace (required)
  --kind, -k         Resource kind: Deployment, StatefulSet, DaemonSet, CronJob (required)
  --action, -a       Action: CREATE, DELETE, ROLL_OUT, SCALE (required)
  --api-url          Use API mode: URL of freeze-operator API (e.g. http://localhost:8082)
  --json             Output as JSON
  --help, -h         Show help

Modes:
  Direct (default)   Uses kubeconfig to query Kubernetes API directly
  API (--api-url)    Calls the operator's CI Helper API endpoint

Environment:
  FREEZE_API_URL     Default API URL (overridden by --api-url)
`)
	os.Exit(0)
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" {
		usage()
	}

	if os.Args[1] != "can-i" {
		fmt.Fprintf(os.Stderr, "Unknown command: %s\nRun 'kfo --help' for usage.\n", os.Args[1])
		os.Exit(2)
	}

	var namespace, kind, action, apiURL string
	var jsonOutput bool

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--namespace", "-n":
			i++
			namespace = args[i]
		case "--kind", "-k":
			i++
			kind = args[i]
		case "--action", "-a":
			i++
			action = args[i]
		case "--api-url":
			i++
			apiURL = args[i]
		case "--json":
			jsonOutput = true
		case "-h", "--help":
			usage()
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\n", args[i])
			os.Exit(2)
		}
	}

	if namespace == "" || kind == "" || action == "" {
		fmt.Fprintln(os.Stderr, "Error: --namespace, --kind, and --action are required")
		os.Exit(2)
	}

	if apiURL == "" {
		apiURL = os.Getenv("FREEZE_API_URL")
	}

	var resp apiResponse
	var err error

	if apiURL != "" {
		resp, err = evalViaAPI(apiURL, namespace, kind, action)
	} else {
		resp, err = evalDirect(namespace, kind, action)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(2)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(resp)
	} else {
		printHuman(resp, namespace, kind, action)
	}

	if !resp.Allow {
		os.Exit(1)
	}
}

func evalViaAPI(apiURL, namespace, kind, action string) (apiResponse, error) {
	url := fmt.Sprintf("%s/v1/evaluate?namespace=%s&kind=%s&action=%s",
		strings.TrimRight(apiURL, "/"), namespace, kind, action)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return apiResponse{}, err
	}

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return apiResponse{}, fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	var resp apiResponse
	if err := json.NewDecoder(httpResp.Body).Decode(&resp); err != nil {
		return apiResponse{}, fmt.Errorf("failed to parse API response: %w", err)
	}

	if resp.Error != "" {
		return apiResponse{}, fmt.Errorf("API error: %s", resp.Error)
	}

	return resp, nil
}

func evalDirect(namespace, kind, action string) (apiResponse, error) {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return apiResponse{}, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return apiResponse{}, fmt.Errorf("failed to create client: %w", err)
	}

	parsedKind, ok := parseKind(kind)
	if !ok {
		return apiResponse{}, fmt.Errorf("unsupported kind: %s (valid: Deployment, StatefulSet, DaemonSet, CronJob)", kind)
	}

	parsedAction, ok := parseAction(action)
	if !ok {
		return apiResponse{}, fmt.Errorf("unsupported action: %s (valid: CREATE, DELETE, ROLL_OUT, SCALE)", action)
	}

	now := time.Now().UTC()
	in := policy.Input{
		Now:       now,
		Namespace: namespace,
		Kind:      parsedKind,
		Action:    parsedAction,
	}

	eval := &policy.Evaluator{Client: cl}
	dec, err := eval.Evaluate(context.Background(), in)
	if err != nil {
		return apiResponse{}, fmt.Errorf("evaluation failed: %w", err)
	}

	resp := apiResponse{
		Allow:       dec.Allowed,
		EvaluatedAt: now.Format(time.RFC3339),
	}

	if !dec.Allowed {
		resp.Reason = dec.Reason
		if dec.MatchedPolicy != nil {
			resp.MatchedPolicy = dec.MatchedPolicy.Name
			resp.PolicyKind = string(dec.MatchedPolicy.Kind)
		}
		if dec.FreezeEndTime != nil {
			t := dec.FreezeEndTime.Format(time.RFC3339)
			resp.FreezeEndTime = &t
		}
		if dec.NextAllowedTime != nil {
			t := dec.NextAllowedTime.Format(time.RFC3339)
			resp.NextAllowed = &t
		}
	}

	return resp, nil
}

func parseKind(s string) (freezev1alpha1.TargetKind, bool) {
	switch freezev1alpha1.TargetKind(s) {
	case freezev1alpha1.TargetKindDeployment:
		return freezev1alpha1.TargetKindDeployment, true
	case freezev1alpha1.TargetKindStatefulSet:
		return freezev1alpha1.TargetKindStatefulSet, true
	case freezev1alpha1.TargetKindDaemonSet:
		return freezev1alpha1.TargetKindDaemonSet, true
	case freezev1alpha1.TargetKindCronJob:
		return freezev1alpha1.TargetKindCronJob, true
	}
	return "", false
}

func parseAction(s string) (freezev1alpha1.Action, bool) {
	switch freezev1alpha1.Action(s) {
	case freezev1alpha1.ActionCreate:
		return freezev1alpha1.ActionCreate, true
	case freezev1alpha1.ActionDelete:
		return freezev1alpha1.ActionDelete, true
	case freezev1alpha1.ActionRollout:
		return freezev1alpha1.ActionRollout, true
	case freezev1alpha1.ActionScale:
		return freezev1alpha1.ActionScale, true
	}
	return "", false
}

func printHuman(resp apiResponse, ns, kind, action string) {
	if resp.Allow {
		fmt.Printf("✅ ALLOWED — %s/%s %s in namespace %s\n", kind, action, action, ns)
	} else {
		fmt.Printf("❌ DENIED — %s %s in namespace %s\n", kind, action, ns)
		if resp.Reason != "" {
			fmt.Printf("   Reason:  %s\n", resp.Reason)
		}
		if resp.MatchedPolicy != "" {
			fmt.Printf("   Policy:  %s (%s)\n", resp.MatchedPolicy, resp.PolicyKind)
		}
		if resp.FreezeEndTime != nil {
			fmt.Printf("   Ends at: %s\n", *resp.FreezeEndTime)
		}
	}
}
