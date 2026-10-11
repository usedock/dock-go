package dock_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	dock "github.com/usedock/dock-go"
)

func TestCreateSendsConnectors(t *testing.T) {
	c, got := serve(t, 201, `{"ok":true,"data":{"id":"d1","name":"n","state":"ready","connectors":["slack","linear"]}}`)
	d, err := c.Docks.Create(context.Background(), dock.CreateParams{WorkspaceID: "w", Connectors: []string{"slack", "linear"}})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Connectors, []string{"slack", "linear"}) {
		t.Fatalf("dock connectors %v", d.Connectors)
	}
	if !reflect.DeepEqual(got.Body["connectors"], []any{"slack", "linear"}) {
		t.Fatalf("body %v", got.Body)
	}
}

func TestCreateOmitsEmptyConnectors(t *testing.T) {
	c, got := serve(t, 201, dockJSON)
	if _, err := c.Docks.Create(context.Background(), dock.CreateParams{WorkspaceID: "w"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Body["connectors"]; ok {
		t.Fatalf("connectors sent: %v", got.Body)
	}
}

func TestCreateConnectorErrors(t *testing.T) {
	cases := []struct {
		status int
		code   string
		want   string
	}{
		{409, "connector_not_connected", dock.CodeConnectorNotConnected},
		{403, "connector_admin_only", dock.CodeConnectorAdminOnly},
		{400, "image_unknown", dock.CodeImageUnknown},
		{400, "image_unavailable", dock.CodeImageUnavailable},
	}
	for _, tc := range cases {
		c, _ := serve(t, tc.status, `{"ok":false,"code":"`+tc.code+`","message":"m"}`)
		_, err := c.Docks.Create(context.Background(), dock.CreateParams{WorkspaceID: "w"})
		var apiErr *dock.Error
		if !errors.As(err, &apiErr) || apiErr.Code != tc.want || apiErr.Status != tc.status {
			t.Fatalf("%s: err %v", tc.code, err)
		}
	}
}

func TestConnectorsList(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":[
		{"kind":"slack","type":"tool","status":"connected","accountLabel":"acme","scopes":["chat:write"],"oauthAvailable":true,"readOnly":false,"updatedAt":"2026-01-01T00:00:00.000Z"},
		{"kind":"github","type":"github","status":"not_connected","accountLabel":null,"scopes":[],"oauthAvailable":false,"readOnly":true,"updatedAt":null}]}`)
	cs, err := c.Connectors.List(context.Background(), "w 1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" || got.Path != "/v1/workspaces/w 1/connectors" {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
	if len(cs) != 2 || cs[0].Kind != "slack" || cs[0].Status != "connected" || cs[0].AccountLabel == nil || *cs[0].AccountLabel != "acme" || !cs[0].OAuthAvailable {
		t.Fatalf("connectors %+v", cs)
	}
	if cs[1].Kind != "github" || !cs[1].ReadOnly || cs[1].AccountLabel != nil || cs[1].UpdatedAt != nil {
		t.Fatalf("github %+v", cs[1])
	}
}

func TestConnectorsConnect(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":{"kind":"linear","type":"tool","status":"connected","accountLabel":"me","scopes":[],"oauthAvailable":false,"readOnly":false,"updatedAt":"2026-01-01T00:00:00.000Z"}}`)
	cn, err := c.Connectors.Connect(context.Background(), "w1", "linear", "lin_tok")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "PUT" || got.Path != "/v1/workspaces/w1/connectors/linear" || got.Body["token"] != "lin_tok" {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Body)
	}
	if cn.Kind != "linear" || cn.Status != "connected" {
		t.Fatalf("connector %+v", cn)
	}
}

func TestConnectorsConnectError(t *testing.T) {
	c, _ := serve(t, 403, `{"ok":false,"code":"forbidden","message":"admins only"}`)
	_, err := c.Connectors.Connect(context.Background(), "w1", "slack", "t")
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Status != 403 {
		t.Fatalf("err %v", err)
	}
}

func TestConnectorsDisconnect(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":{"kind":"slack","status":"not_connected"}}`)
	cn, err := c.Connectors.Disconnect(context.Background(), "w1", "slack")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "DELETE" || got.Path != "/v1/workspaces/w1/connectors/slack" {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
	if cn.Kind != "slack" || cn.Status != "not_connected" {
		t.Fatalf("connector %+v", cn)
	}
}

func TestImagesList(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":[{"name":"ubuntu-24.04","description":"d","arch":["amd64","arm64"],"sizeMiB":512,"contents":"base","available":true},{"name":"browser","description":"b","arch":["amd64"],"sizeMiB":900,"contents":"chromium","available":false}]}`)
	imgs, err := c.Images.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" || got.Path != "/v1/images" {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
	if len(imgs) != 2 || imgs[0].Name != "ubuntu-24.04" || !reflect.DeepEqual(imgs[0].Arch, []string{"amd64", "arm64"}) || imgs[0].SizeMiB != 512 || !imgs[0].Available || imgs[1].Available {
		t.Fatalf("images %+v", imgs)
	}
}
