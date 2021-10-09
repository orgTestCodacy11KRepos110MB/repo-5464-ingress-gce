/*
Copyright 2019 The Kubernetes Authors.

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

// Generator for GCE compute wrapper code. You must regenerate the code after
// modifying this file:
//
//   $ ./hack/update_codegen.sh

package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	compositemeta "k8s.io/ingress-gce/pkg/composite/meta"
)

const (
	gofmt = "gofmt"
)

// gofmtContent runs "gofmt" on the given contents.
func gofmtContent(r io.Reader) string {
	cmd := exec.Command(gofmt, "-s")
	out := &bytes.Buffer{}
	cmd.Stdin = r
	cmd.Stdout = out
	cmdErr := &bytes.Buffer{}
	cmd.Stderr = cmdErr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, cmdErr.String())
		panic(err)
	}
	return out.String()
}

func genHeader(wr io.Writer) {
	const text = `/*
Copyright {{.Year}} The Kubernetes Authors.

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
// This file was generated by "./hack/update-codegen.sh". Do not edit directly.
// directly.

package composite
import (
	"fmt"

	"k8s.io/klog"
	computealpha "google.golang.org/api/compute/v0.alpha"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/meta"
	"github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud/filter"
	cloudprovider "github.com/GoogleCloudPlatform/k8s-cloud-provider/pkg/cloud"
	compositemetrics "k8s.io/ingress-gce/pkg/composite/metrics"
	"k8s.io/legacy-cloud-providers/gce"
)
`
	tmpl := template.Must(template.New("header").Parse(text))
	values := map[string]string{
		"Year": fmt.Sprintf("%v", time.Now().Year()),
	}
	if err := tmpl.Execute(wr, values); err != nil {
		panic(err)
	}

	fmt.Fprintf(wr, "\n\n")
}

func genTestHeader(wr io.Writer) {
	const text = `/*
Copyright {{.Year}} The Kubernetes Authors.

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
// This file was generated by "./hack/update-codegen.sh". Do not edit directly.
// directly.

package composite
import (
	"fmt"
	"reflect"
	"testing"

	"github.com/kr/pretty"
	computealpha "google.golang.org/api/compute/v0.alpha"
	computebeta "google.golang.org/api/compute/v0.beta"
	compute "google.golang.org/api/compute/v1"
)
`
	tmpl := template.Must(template.New("testHeader").Parse(text))
	values := map[string]string{
		"Year": fmt.Sprintf("%v", time.Now().Year()),
	}
	if err := tmpl.Execute(wr, values); err != nil {
		panic(err)
	}

	fmt.Fprintf(wr, "\n\n")
}

// genTypes() generates all of the composite structs.
func genTypes(wr io.Writer) {
	const text = `
{{ $backtick := "` + "`" + `" }}
{{- range .All}}
	// {{.Name}} is a composite type wrapping the Alpha, Beta, and GA methods for its GCE equivalent
	type {{.Name}} struct {
		{{- if .IsMainService}}
			// Version keeps track of the intended compute version for this {{.Name}}.
			// Note that the compute API's do not contain this field. It is for our
			// own bookkeeping purposes.
			Version meta.Version {{$backtick}}json:"-"{{$backtick}}
			// Scope keeps track of the intended type of the service (e.g. Global)
			// This is also an internal field purely for bookkeeping purposes
			Scope meta.KeyType {{$backtick}}json:"-"{{$backtick}}
		{{end}}

		{{- range .Fields}}
			{{.Description}}
			{{- if eq .Name "Id"}}
				{{- .Name}} {{.GoType}} {{$backtick}}json:"{{.JsonName}},omitempty,string"{{$backtick}}
			{{- else if .JsonStringOverride}}
				{{- .Name}} {{.GoType}} {{$backtick}}json:"{{.JsonName}},omitempty,string"{{$backtick}}
			{{- else}}
				{{- .Name}} {{.GoType}} {{$backtick}}json:"{{.JsonName}},omitempty"{{$backtick}}
			{{- end}}
		{{- end}}
		{{- if and .IsMainService .HasCRUD}}
			googleapi.ServerResponse {{$backtick}}json:"-"{{$backtick}}
		{{- end}}
		ForceSendFields []string {{$backtick}}json:"-"{{$backtick}}
		NullFields []string {{$backtick}}json:"-"{{$backtick}}
}
{{- end}}
`
	data := struct {
		All []compositemeta.ApiService
	}{compositemeta.AllApiServices}

	tmpl := template.Must(template.New("types").Parse(text))
	if err := tmpl.Execute(wr, data); err != nil {
		panic(err)
	}
}

// genFuncs() generates helper methods attached to composite structs.
// TODO: (shance) Fix force send fields hack
// TODO: (shance) Have To*() Functions set ResourceType and Version fields
// TODO: (shance) Figure out a better solution so that the List() functions don't have to take a meta.Key
// that ignores the name field
func genFuncs(wr io.Writer) {
	const text = `
{{/* regionalKeyFiller denotes the keyword to use when invoking the regional API for the resource*/}}
{{$regionalKeyFiller := ""}}
{{/* globalKeyFiller denotes the keyword to use when invoking the global API for the resource*/}}
{{$globalKeyFiller := ""}}
{{$onlyZonalKeySupported := false}}

{{$All := .All}}
{{$Versions := .Versions}}

{{range $type := $All}}
{{if .IsDefaultZonalService}}
	{{$onlyZonalKeySupported = true}}
{{else if .IsDefaultRegionalService}}
	{{$regionalKeyFiller = ""}}
	{{$globalKeyFiller = "Global"}}
	{{$onlyZonalKeySupported = false}}
{{else}}
	{{$regionalKeyFiller = "Region"}}
	{{$globalKeyFiller = ""}}
	{{$onlyZonalKeySupported = false}}
{{- end}} {{/* IsDefaultZonalService */}}
	{{if .IsMainService}}
		{{if .HasCRUD}}
func Create{{.Name}}(gceCloud *gce.Cloud, key *meta.Key, {{.VarName}} *{{.Name}}) error {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "create", key.Region, key.Zone, string({{.VarName}}.Version))

	{{- if $onlyZonalKeySupported}}
	switch key.Type() {
	case meta.Zonal:
	default:
		return fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}
	{{- end}} {{/* $onlyZonalKeySupported*/}}

	switch {{.VarName}}.Version {
	case meta.VersionAlpha:
		alpha, err := {{.VarName}}.ToAlpha()
		if err != nil {
			return err
		}
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Creating alpha zonal {{.Name}} %v", alpha.Name)
		return mc.Observe(gceCloud.Compute().Alpha{{.GetCloudProviderName}}().Insert(ctx, key, alpha))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Creating alpha region {{.Name}} %v", alpha.Name)
			alpha.Region = key.Region
			return mc.Observe(gceCloud.Compute().Alpha{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Insert(ctx, key, alpha))
		default:
			klog.V(3).Infof("Creating alpha {{.Name}} %v", alpha.Name)
			return mc.Observe(gceCloud.Compute().Alpha{{$globalKeyFiller}}{{.GetCloudProviderName}}().Insert(ctx, key, alpha))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	case meta.VersionBeta:
		beta, err := {{.VarName}}.ToBeta()
		if err != nil {
			return err
		}
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Creating beta zonal {{.Name}} %v", beta.Name)
		return mc.Observe(gceCloud.Compute().Beta{{.GetCloudProviderName}}().Insert(ctx, key, beta))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Creating beta region {{.Name}} %v", beta.Name)
			beta.Region = key.Region
			return mc.Observe(gceCloud.Compute().Beta{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Insert(ctx, key, beta))
		default:
			klog.V(3).Infof("Creating beta {{.Name}} %v", beta.Name)
			return mc.Observe(gceCloud.Compute().Beta{{$globalKeyFiller}}{{.GetCloudProviderName}}().Insert(ctx, key, beta))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	default:
		ga, err := {{.VarName}}.ToGA()
		if err != nil {
			return err
		}
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Creating ga zonal {{.Name}} %v", ga.Name)
		return mc.Observe(gceCloud.Compute().{{.GetCloudProviderName}}().Insert(ctx, key, ga))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Creating ga region {{.Name}} %v", ga.Name)
			ga.Region = key.Region
			return mc.Observe(gceCloud.Compute().{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Insert(ctx, key, ga))
		default:
			klog.V(3).Infof("Creating ga {{.Name}} %v", ga.Name)
			return mc.Observe(gceCloud.Compute().{{$globalKeyFiller}}{{.GetCloudProviderName}}().Insert(ctx, key, ga))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	}
}

{{if .HasUpdate}}
func Update{{.Name}}(gceCloud *gce.Cloud, key *meta.Key, {{.VarName}} *{{.Name}}) error {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "update", key.Region, key.Zone, string({{.VarName}}.Version))

	{{- if $onlyZonalKeySupported}}
	switch key.Type() {
	case meta.Zonal:
	default:
		return fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}

	{{- end}} {{/* $onlyZonalKeySupported*/}}
	switch {{.VarName}}.Version {
	case meta.VersionAlpha:
		alpha, err := {{.VarName}}.ToAlpha()
		if err != nil {
			return err
		}
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Updating alpha zonal {{.Name}} %v", alpha.Name)
		return mc.Observe(gceCloud.Compute().Alpha{{.GetCloudProviderName}}().Update(ctx, key, alpha))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Updating alpha region {{.Name}} %v", alpha.Name)
			return mc.Observe(gceCloud.Compute().Alpha{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Update(ctx, key, alpha))
		default:
			klog.V(3).Infof("Updating alpha {{.Name}} %v", alpha.Name)
			return mc.Observe(gceCloud.Compute().Alpha{{$globalKeyFiller}}{{.GetCloudProviderName}}().Update(ctx, key, alpha))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	case meta.VersionBeta:
		beta, err := {{.VarName}}.ToBeta()
		if err != nil {
			return err
		}
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Updating beta zonal {{.Name}} %v", beta.Name)
		return mc.Observe(gceCloud.Compute().Beta{{.GetCloudProviderName}}().Update(ctx, key, beta))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
		  klog.V(3).Infof("Updating beta region {{.Name}} %v", beta.Name)
			return mc.Observe(gceCloud.Compute().Beta{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Update(ctx, key, beta))
		default:
			klog.V(3).Infof("Updating beta {{.Name}} %v", beta.Name)
			return mc.Observe(gceCloud.Compute().Beta{{$globalKeyFiller}}{{.GetCloudProviderName}}().Update(ctx, key, beta))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	default:
		ga, err := {{.VarName}}.ToGA()
		if err != nil {
			return err
		}
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Updating ga zonal {{.Name}} %v", ga.Name)
		return mc.Observe(gceCloud.Compute().{{.GetCloudProviderName}}().Update(ctx, key, ga))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Updating ga region {{.Name}} %v", ga.Name)
			return mc.Observe(gceCloud.Compute().{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Update(ctx, key, ga))
		default:
			klog.V(3).Infof("Updating ga {{.Name}} %v", ga.Name)
			return mc.Observe(gceCloud.Compute().{{$globalKeyFiller}}{{.GetCloudProviderName}}().Update(ctx, key, ga))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	}
}
{{- end}} {{/*HasUpdate*/}}

func Delete{{.Name}}(gceCloud *gce.Cloud, key *meta.Key, version meta.Version) error {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "delete", key.Region, key.Zone, string(version))

	{{- if $onlyZonalKeySupported}}
	switch key.Type() {
	case meta.Zonal:
	default:
		return fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}
	{{- end}} {{/* $onlyZonalKeySupported*/}}

	switch version {
	case meta.VersionAlpha:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Deleting alpha zonal {{.Name}} %v", key.Name)
		return mc.Observe(gceCloud.Compute().Alpha{{.GetCloudProviderName}}().Delete(ctx, key))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Deleting alpha region {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().Alpha{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Delete(ctx, key))
		default:
			klog.V(3).Infof("Deleting alpha {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().Alpha{{$globalKeyFiller}}{{.GetCloudProviderName}}().Delete(ctx, key))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	case meta.VersionBeta:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Deleting beta zonal {{.Name}} %v", key.Name)
		return mc.Observe(gceCloud.Compute().Beta{{.GetCloudProviderName}}().Delete(ctx, key))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
		  klog.V(3).Infof("Deleting beta region {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().Beta{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Delete(ctx, key))
		default:
		  klog.V(3).Infof("Deleting beta {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().Beta{{$globalKeyFiller}}{{.GetCloudProviderName}}().Delete(ctx, key))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	default:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Deleting ga zonal {{.Name}} %v", key.Name)
		return mc.Observe(gceCloud.Compute().{{.GetCloudProviderName}}().Delete(ctx, key))
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Deleting ga region {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Delete(ctx, key))
		default:
			klog.V(3).Infof("Deleting ga {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().{{$globalKeyFiller}}{{.GetCloudProviderName}}().Delete(ctx, key))
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	}
}

func Get{{.Name}}(gceCloud *gce.Cloud, key *meta.Key, version meta.Version) (*{{.Name}}, error) {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "get", key.Region, key.Zone, string(version))

	var gceObj interface{}
	var err error

	{{- if $onlyZonalKeySupported}}
	switch key.Type() {
	case meta.Zonal:
	default:
		return nil, fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}

	{{- end}} {{/* $onlyZonalKeySupported*/}}
	switch version {
	case meta.VersionAlpha:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Getting alpha zonal {{.Name}} %v", key.Name)
		gceObj, err = gceCloud.Compute().Alpha{{.GetCloudProviderName}}().Get(ctx, key)
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Getting alpha region {{.Name}} %v", key.Name)
			gceObj, err = gceCloud.Compute().Alpha{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Get(ctx, key)
		default:
		  	klog.V(3).Infof("Getting alpha {{.Name}} %v", key.Name)
			gceObj, err = gceCloud.Compute().Alpha{{$globalKeyFiller}}{{.GetCloudProviderName}}().Get(ctx, key)
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	case meta.VersionBeta:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Getting beta zonal {{.Name}} %v", key.Name)
		gceObj, err = gceCloud.Compute().Beta{{.GetCloudProviderName}}().Get(ctx, key)
	{{else}}
		switch key.Type() {
		case meta.Regional:
		  	klog.V(3).Infof("Getting beta region {{.Name}} %v", key.Name)
			gceObj, err = gceCloud.Compute().Beta{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Get(ctx, key)
		default:
		  	klog.V(3).Infof("Getting beta {{.Name}} %v", key.Name)
			gceObj, err = gceCloud.Compute().Beta{{$globalKeyFiller}}{{.GetCloudProviderName}}().Get(ctx, key)
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	default:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Getting ga zonal {{.Name}} %v", key.Name)
		gceObj, err = gceCloud.Compute().{{.GetCloudProviderName}}().Get(ctx, key)
	{{- else}}
		switch key.Type() {
		case meta.Regional:
      		klog.V(3).Infof("Getting ga region {{.Name}} %v", key.Name)
		gceObj, err = gceCloud.Compute().{{$regionalKeyFiller}}{{.GetCloudProviderName}}().Get(ctx, key)
		default:
      		klog.V(3).Infof("Getting ga {{.Name}} %v", key.Name)
		gceObj, err = gceCloud.Compute().{{$globalKeyFiller}}{{.GetCloudProviderName}}().Get(ctx, key)
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	}
	if err != nil {
		return nil, mc.Observe(err)
	}
	compositeType, err := to{{.Name}}(gceObj)
  	if err != nil {
    	return nil, err
  	}

	{{- if $onlyZonalKeySupported}}
	compositeType.Scope = meta.Zonal
	{{- else if .IsDefaultRegionalService}}
	if key.Type() == meta.Regional {
		compositeType.Scope = meta.Regional
	}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
  	compositeType.Version = version
  	return compositeType, nil
}

func List{{.GetCloudProviderName}}(gceCloud *gce.Cloud, key *meta.Key, version meta.Version) ([]*{{.Name}}, error) {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "list", key.Region, key.Zone, string(version))

	var gceObjs interface{}
	var err error

	{{- if $onlyZonalKeySupported}}
	switch key.Type() {
	case meta.Zonal:
	default:
		return nil, fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}

	{{- end}} {{/* $onlyZonalKeySupported*/}}
	switch version {
	case meta.VersionAlpha:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Listing alpha zone{{.Name}}")
		gceObjs, err = gceCloud.Compute().Alpha{{.GetCloudProviderName}}().List(ctx, key.Zone, filter.None)
	{{- else}}
		switch key.Type() {
		case meta.Regional:
			klog.V(3).Infof("Listing alpha region {{.Name}}")
			gceObjs, err = gceCloud.Compute().Alpha{{$regionalKeyFiller}}{{.GetCloudProviderName}}().List(ctx, key.Region, filter.None)
		default:
		  	klog.V(3).Infof("Listing alpha {{.Name}}")
			gceObjs, err = gceCloud.Compute().Alpha{{$globalKeyFiller}}{{.GetCloudProviderName}}().List(ctx, filter.None)
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	case meta.VersionBeta:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Listing beta zone{{.Name}}")
		gceObjs, err = gceCloud.Compute().Beta{{.GetCloudProviderName}}().List(ctx, key.Zone, filter.None)
	{{- else}}
		switch key.Type() {
		case meta.Regional:
		  	klog.V(3).Infof("Listing beta region {{.Name}}")
			gceObjs, err = gceCloud.Compute().Beta{{$regionalKeyFiller}}{{.GetCloudProviderName}}().List(ctx, key.Region, filter.None)
		default:
		  	klog.V(3).Infof("Listing beta {{.Name}}")
			gceObjs, err = gceCloud.Compute().Beta{{$globalKeyFiller}}{{.GetCloudProviderName}}().List(ctx, filter.None)
		}
	{{- end}} {{/* $onlyZonalKeySupported*/}}
	default:
	{{- if $onlyZonalKeySupported}}
		klog.V(3).Infof("Listing ga zone{{.Name}}")
		gceObjs, err = gceCloud.Compute().{{.GetCloudProviderName}}().List(ctx, key.Zone, filter.None)
    {{- else}}
		switch key.Type() {
		case meta.Regional:
 			klog.V(3).Infof("Listing ga region {{.Name}}")
			gceObjs, err = gceCloud.Compute().{{$regionalKeyFiller}}{{.GetCloudProviderName}}().List(ctx, key.Region, filter.None)
		default:
 			klog.V(3).Infof("Listing ga {{.Name}}")
			gceObjs, err = gceCloud.Compute().{{$globalKeyFiller}}{{.GetCloudProviderName}}().List(ctx, filter.None)
		}
    {{- end}} {{/* $onlyZonalKeySupported*/}}
	}
	if err != nil {
		return nil, mc.Observe(err)
	}

	compositeObjs, err := to{{.Name}}List(gceObjs)
	if err != nil {
		return nil, err
	}
	for _, obj := range compositeObjs {
		obj.Version = version
	}
	return compositeObjs, nil
}

{{if .IsGroupResourceService}}
func {{.GetGroupResourceInfo.AttachFuncName}}(gceCloud *gce.Cloud, key *meta.Key, version meta.Version, req *{{.GetGroupResourceInfo.AttachReqName}}) error {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "attach", key.Region, key.Zone, string(version))

	switch key.Type() {
	case meta.Zonal:
	default:
      return fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}

	switch version {
	case meta.VersionAlpha:
		alphareq, err := req.ToAlpha()
		if err != nil {
			return err
		}
        klog.V(3).Infof("Attaching to alpha zonal {{.Name}} %v", key.Name)
        return mc.Observe(gceCloud.Compute().Alpha{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.AttachFuncName}}(ctx, key, alphareq))
	case meta.VersionBeta:
		betareq, err := req.ToBeta()
		if err != nil {
			return err
		}
		klog.V(3).Infof("Attaching to beta zonal {{.Name}} %v", key.Name)
		return mc.Observe(gceCloud.Compute().Beta{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.AttachFuncName}}(ctx, key, betareq))
	default:
		gareq, err := req.ToGA()
		if err != nil {
			return err
		}
		  klog.V(3).Infof("Attaching to ga zonal {{.Name}} %v", key.Name)
			return mc.Observe(gceCloud.Compute().{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.AttachFuncName}}(ctx, key, gareq))
	}
}

func {{.GetGroupResourceInfo.DetachFuncName}}(gceCloud *gce.Cloud, key *meta.Key, version meta.Version, req *{{.GetGroupResourceInfo.DetachReqName}}) error {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "detach", key.Region, key.Zone, string(version))

	switch key.Type() {
	case meta.Zonal:
	default:
      return fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}

	switch version {
	case meta.VersionAlpha:
		alphareq, err := req.ToAlpha()
		if err != nil {
			return err
		}
        klog.V(3).Infof("Detaching from alpha zonal {{.Name}} %v", key.Name)
        return mc.Observe(gceCloud.Compute().Alpha{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.DetachFuncName}}(ctx, key, alphareq))
	case meta.VersionBeta:
		betareq, err := req.ToBeta()
		if err != nil {
			return err
		}
		klog.V(3).Infof("Detaching from beta zonal {{.Name}} %v", key.Name)
		return mc.Observe(gceCloud.Compute().Beta{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.DetachFuncName}}(ctx, key, betareq))
	default:
		gareq, err := req.ToGA()
		if err != nil {
			return err
		}
		klog.V(3).Infof("Detaching from ga zonal {{.Name}} %v", key.Name)
		return mc.Observe(gceCloud.Compute().{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.DetachFuncName}}(ctx, key, gareq))
	}
}

func {{.GetGroupResourceInfo.ListFuncName}}(gceCloud *gce.Cloud, key *meta.Key, version meta.Version, req *{{.GetGroupResourceInfo.ListReqName}}) ([]*{{.GetGroupResourceInfo.ListRespName}}, error) {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "list", key.Region, key.Zone, string(version))

	var gceObjs interface{}
	var err error

	switch key.Type() {
	case meta.Zonal:
	default:
            return nil, fmt.Errorf("Key %v not valid for zonal resource {{.Name}} %v", key, key.Name)
	}

	switch version {
	case meta.VersionAlpha:
		alphareq, reqerr := req.ToAlpha()
		if reqerr != nil {
			return nil, reqerr
		}
        klog.V(3).Infof("Listing alpha zonal {{.Name}} %v", key.Name)
        gceObjs, err = gceCloud.Compute().Alpha{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.ListFuncName}}(ctx, key, alphareq, filter.None)
	case meta.VersionBeta:
		betareq, reqerr := req.ToBeta()
		if reqerr != nil {
			return nil, reqerr
		}
		klog.V(3).Infof("Listing beta zonal {{.Name}} %v", key.Name)
		gceObjs, err = gceCloud.Compute().Beta{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.ListFuncName}}(ctx, key, betareq, filter.None)
	default:
		gareq, reqerr := req.ToGA()
		if reqerr != nil {
			return nil, reqerr
		}
		  klog.V(3).Infof("Listing ga zonal {{.Name}} %v", key.Name)
			gceObjs, err = gceCloud.Compute().{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.ListFuncName}}(ctx, key, gareq, filter.None)
	}
	if err != nil {
		return nil, mc.Observe(err)
	}

	compositeObjs, err := to{{.GetGroupResourceInfo.ListRespName}}List(gceObjs)
	if err != nil {
		return nil, err
	}
	for _, obj := range compositeObjs {
		obj.Version = version
	}
	return compositeObjs, nil
}

func {{.GetGroupResourceInfo.AggListFuncName}}{{.GetGroupResourceInfo.AggListRespName}}(gceCloud *gce.Cloud, version meta.Version) (map[*meta.Key]*{{.GetGroupResourceInfo.AggListRespName}}, error) {
	ctx, cancel := cloudprovider.ContextWithCallTimeout()
	defer cancel()
	mc := compositemetrics.NewMetricContext("{{.Name}}", "aggregateList", "", "", string(version))

	compositeMap := make(map[*meta.Key]*{{.GetGroupResourceInfo.AggListRespName}})
	var gceObjs interface{}

	switch version {
	case meta.VersionAlpha:
		klog.V(3).Infof("Aggregate List of alpha zonal {{.Name}}")
		alphaMap, err := gceCloud.Compute().Alpha{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.AggListFuncName}}(ctx, filter.None)
		if err != nil {
			return nil, mc.Observe(err)
		}
		// Convert from map to list
		alphaList := []*computealpha.{{.GetGroupResourceInfo.AggListRespName}}{}
		for _, val := range alphaMap {
			alphaList = append(alphaList, val...)
		}
		gceObjs = alphaList
	case meta.VersionBeta:
		klog.V(3).Infof("Aggregate List of beta zonal {{.Name}}")
		betaMap, err := gceCloud.Compute().Beta{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.AggListFuncName}}(ctx, filter.None)
		if err != nil {
			return nil, mc.Observe(err)
		}
		// Convert from map to list
		betaList := []*computebeta.{{.GetGroupResourceInfo.AggListRespName}}{}
		for _, val := range betaMap {
			betaList = append(betaList, val...)
		}
		gceObjs = betaList
	default:
		klog.V(3).Infof("Aggregate List of ga zonal {{.Name}}")
		gaMap, err := gceCloud.Compute().{{.GetCloudProviderName}}().{{.GetGroupResourceInfo.AggListFuncName}}(ctx, filter.None)
		if err != nil {
			return nil, mc.Observe(err)
		}
		// Convert from map to list
		gaList := []*compute.{{.GetGroupResourceInfo.AggListRespName}}{}
		for _, val := range gaMap {
			gaList = append(gaList, val...)
		}
		gceObjs = gaList
	}
	compositeObjs, err := to{{.GetGroupResourceInfo.AggListRespName}}List(gceObjs)
	if err != nil {
		return nil, err
	}
	for _, obj := range compositeObjs {
		obj.Version = version
		resourceID, err := cloudprovider.ParseResourceURL(obj.SelfLink)
		if err != nil || resourceID == nil || resourceID.Key == nil {
			klog.Errorf("Failed to parse SelfLink - %s for obj %v, err %v", obj.SelfLink, obj, err)
			continue
		}
		compositeMap[resourceID.Key] = obj
	}

	return compositeMap, nil
}
{{end}} {{/*IsGroupResourceService*/}}
{{- end}} {{/*HasCRUD*/}}

// to{{.Name}}List converts a list of compute alpha, beta or GA
// {{.Name}} into a list of our composite type.
func to{{.Name}}List(objs interface{}) ([]*{{.Name}}, error) {
	result := []*{{.Name}}{}

	err := copyViaJSON(&result, objs)
	if err != nil {
		return nil, fmt.Errorf("could not copy object %v to %T via JSON: %v", objs, result, err)
	}
	return result, nil
}

// to{{.Name}} is for package internal use only (not type-safe).
func to{{.Name}}(obj interface{}) (*{{.Name}}, error) {
	x := &{{.Name}}{}
	err := copyViaJSON(x, obj)
	if err != nil {
		return nil, fmt.Errorf("could not copy object %+v to %T via JSON: %v", obj, x, err)
	}
	return x, nil
}

// Users external to the package need to pass in the correct type to create a
// composite.

{{- range $version, $extension := $.Versions}}

// {{$version}}To{{$type.Name}} convert to a composite type.
func {{$version}}To{{$type.Name}}(obj *compute{{$extension}}.{{$type.Name}}) (*{{$type.Name}}, error) {
	x := &{{$type.Name}}{}
	err := copyViaJSON(x, obj)
	if err != nil {
		return nil, fmt.Errorf("could not copy object %+v to %T via JSON: %v", obj, x, err)
	}
	return x, nil
}

{{- end}} {{/* range versions */}}

{{- range $version, $extension := $.Versions}}
{{$lower := $version | ToLower}}
// To{{$version}} converts our composite type into an {{$lower}} type.
// This {{$lower}} type can be used in GCE API calls.
func ({{$type.VarName}} *{{$type.Name}}) To{{$version}}() (*compute{{$extension}}.{{$type.Name}}, error) {
	{{$lower}} := &compute{{$extension}}.{{$type.Name}}{}
	err := copyViaJSON({{$lower}}, {{$type.VarName}})
	if err != nil {
		return nil, fmt.Errorf("error converting %T to compute {{$lower}} type via JSON: %v", {{$type.VarName}}, err)
	}

	{{- if eq $type.Name "BackendService"}}
	// Set force send fields. This is a temporary hack.
	if {{$lower}}.CdnPolicy != nil { 
		if {{$lower}}.CdnPolicy.CacheKeyPolicy != nil {
		{{$lower}}.CdnPolicy.CacheKeyPolicy.ForceSendFields = []string{"IncludeHost", "IncludeProtocol", "IncludeQueryString", "QueryStringBlacklist", "QueryStringWhitelist"}
		}
		{{$lower}}.CdnPolicy.ForceSendFields = append({{$type.VarName}}.CdnPolicy.ForceSendFields, []string{"NegativeCaching", "RequestCoalescing","SignedUrlCacheMaxAgeSec","ServeWhileStale"}...)
	}
	if {{$lower}}.Iap != nil {
		{{$lower}}.Iap.ForceSendFields = []string{"Enabled", "Oauth2ClientId", "Oauth2ClientSecret"}
	}
	if {{$lower}}.LogConfig != nil {
		{{$lower}}.LogConfig.ForceSendFields = []string{"Enable"}
		if {{$lower}}.LogConfig.Enable {
			{{$lower}}.LogConfig.ForceSendFields = []string{"Enable", "SampleRate"}
		}
	}
	{{- end}}

	return {{$lower}}, nil
}
{{- end}} {{/* range versions */}}
{{- end}} {{/* isMainType */}}
{{- end}} {{/* range */}}
`
	data := struct {
		All      []compositemeta.ApiService
		Versions map[string]string
	}{compositemeta.AllApiServices, compositemeta.Versions}

	funcMap := template.FuncMap{
		"ToLower": strings.ToLower,
	}

	tmpl := template.Must(template.New("funcs").Funcs(funcMap).Parse(text))
	if err := tmpl.Execute(wr, data); err != nil {
		panic(err)
	}
}

// genTests() generates all of the tests
func genTests(wr io.Writer) {
	const text = `
{{ $All := .All}}
{{range $type := $All}}
		{{- if .IsMainService}}
			func Test{{.Name}}(t *testing.T) {
	// Use reflection to verify that our composite type contains all the
	// same fields as the alpha type.
	compositeType := reflect.TypeOf({{.Name}}{})
	alphaType := reflect.TypeOf(computealpha.{{.Name}}{})
	betaType := reflect.TypeOf(computebeta.{{.Name}}{})
	gaType := reflect.TypeOf(compute.{{.Name}}{})

	// For the composite type, remove the Version field from consideration
	compositeTypeNumFields := compositeType.NumField() - 2
	if compositeTypeNumFields != alphaType.NumField() {
		t.Fatalf("%v should contain %v fields. Got %v", alphaType.Name(), alphaType.NumField(), compositeTypeNumFields)
	}

  // Compare all the fields by doing a lookup since we can't guarantee that they'll be in the same order
	// Make sure that composite type is strictly alpha fields + internal bookkeeping
	for i := 2; i < compositeType.NumField(); i++ {
		lookupField, found := alphaType.FieldByName(compositeType.Field(i).Name)
		if !found {
			t.Fatal(fmt.Errorf("Field %v not present in alpha type %v", compositeType.Field(i), alphaType))
		}
		if err := compareFields(compositeType.Field(i), lookupField); err != nil {
			t.Fatal(err)
		}
	}

  // Verify that all beta fields are in composite type
	if err := typeEquality(betaType, compositeType, false); err != nil {
		t.Fatal(err)
	}

	// Verify that all GA fields are in composite type
	if err := typeEquality(gaType, compositeType, false); err != nil {
		t.Fatal(err)
	}
}

// TODO: these tests don't do anything as they are currently structured.
// func TestTo{{.Name}}(t *testing.T)

{{range $version, $extension := $.Versions}}
func Test{{$type.Name}}To{{$version}}(t *testing.T) {
	composite := {{$type.Name}}{}
	expected := &compute{{$extension}}.{{$type.Name}}{}
	result, err := composite.To{{$version}}()
	if err != nil {
		t.Fatalf("{{$type.Name}}.To{{$version}}() error: %v", err)
	}

	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("{{$type.Name}}.To{{$version}}() = \ninput = %s\n%s\nwant = \n%s", pretty.Sprint(composite), pretty.Sprint(result), pretty.Sprint(expected))
	}
}
{{- end}}
{{- else}}

func Test{{.Name}}(t *testing.T) {
	compositeType := reflect.TypeOf({{.Name}}{})
	alphaType := reflect.TypeOf(computealpha.{{.Name}}{})
	if err := typeEquality(compositeType, alphaType, true); err != nil {
		t.Fatal(err)
	}
}
{{- end}}
{{- end}}
`
	data := struct {
		All      []compositemeta.ApiService
		Versions map[string]string
	}{compositemeta.AllApiServices, compositemeta.Versions}

	funcMap := template.FuncMap{
		"ToLower": strings.ToLower,
	}

	tmpl := template.Must(template.New("tests").Funcs(funcMap).Parse(text))
	if err := tmpl.Execute(wr, data); err != nil {
		panic(err)
	}
}

func main() {
	out := &bytes.Buffer{}
	testOut := &bytes.Buffer{}

	genHeader(out)
	genTypes(out)
	genFuncs(out)

	genTestHeader(testOut)
	genTests(testOut)

	var err error
	err = ioutil.WriteFile("./pkg/composite/gen.go", []byte(gofmtContent(out)), 0644)
	//err = ioutil.WriteFile("./pkg/composite/composite.go", []byte(out.String()), 0644)
	if err != nil {
		panic(err)
	}
	err = ioutil.WriteFile("./pkg/composite/gen_test.go", []byte(gofmtContent(testOut)), 0644)
	if err != nil {
		panic(err)
	}
}
