package dock_test

import (
	"context"
	"reflect"
	"testing"

	dock "github.com/usedock/dock-go"
)

const envJSON = `{"id":"e1","organizationId":"o1","name":"base","isDefault":true,"currentVersion":3,"secretsAccess":"admins","providerKeys":"gateway","egressAllow":null}`

func TestEnvironmentsListReadsProviderKeys(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":[`+envJSON+`]}`)
	envs, err := c.Environments.List(context.Background(), "o1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" || got.Path != "/v1/environments" || got.Query != "organizationId=o1" {
		t.Fatalf("%s %s?%s", got.Method, got.Path, got.Query)
	}
	if len(envs) != 1 || envs[0].ProviderKeys != dock.ProviderKeysGateway || envs[0].EgressAllow != nil {
		t.Fatalf("envs %+v", envs)
	}
}

func TestEnvironmentsGet(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":`+envJSON+`}`)
	e, err := c.Environments.Get(context.Background(), "base", "o1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/v1/environments/base" || got.Query != "organizationId=o1" || e.ProviderKeys != dock.ProviderKeysGateway {
		t.Fatalf("%s?%s %+v", got.Path, got.Query, e)
	}
}

func TestEnvironmentsUpdateSendsProviderKeys(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":{"id":"e1","organizationId":"o1","name":"base","providerKeys":"plain"}}`)
	f := false
	e, err := c.Environments.Update(context.Background(), "base", "o1", dock.UpdateEnvironmentParams{ProviderKeys: dock.ProviderKeysPlain, PassGithub: &f})
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "PATCH" || got.Path != "/v1/environments/base" || got.Body["providerKeys"] != "plain" || got.Body["passGithub"] != false {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Body)
	}
	if _, ok := got.Body["name"]; ok {
		t.Fatalf("unset fields sent: %v", got.Body)
	}
	if e.ProviderKeys != dock.ProviderKeysPlain {
		t.Fatalf("env %+v", e)
	}
}

func TestEnvironmentsUpdateEgressAllow(t *testing.T) {
	cases := []struct {
		name    string
		params  dock.UpdateEnvironmentParams
		present bool
		want    any
	}{
		{"set", dock.UpdateEnvironmentParams{EgressAllow: []string{"api.example.com", "*.example.org"}}, true, []any{"api.example.com", "*.example.org"}},
		{"clear", dock.UpdateEnvironmentParams{ClearEgressAllow: true}, true, nil},
		{"unset", dock.UpdateEnvironmentParams{ProviderKeys: dock.ProviderKeysGateway}, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, got := serve(t, 200, `{"ok":true,"data":`+envJSON+`}`)
			if _, err := c.Environments.Update(context.Background(), "e1", "", tc.params); err != nil {
				t.Fatal(err)
			}
			v, ok := got.Body["egressAllow"]
			if ok != tc.present || !reflect.DeepEqual(v, tc.want) {
				t.Fatalf("egressAllow present=%v value=%#v, body %v", ok, v, got.Body)
			}
		})
	}
}

func TestEnvironmentGetReadsEgressAllow(t *testing.T) {
	c, _ := serve(t, 200, `{"ok":true,"data":{"id":"e1","organizationId":"o1","name":"base","providerKeys":"plain","egressAllow":["api.example.com"],"varNames":["A"]}}`)
	e, err := c.Environments.Get(context.Background(), "e1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.EgressAllow, []string{"api.example.com"}) || e.ProviderKeys != dock.ProviderKeysPlain || !reflect.DeepEqual(e.VarNames, []string{"A"}) {
		t.Fatalf("env %+v", e)
	}
}
