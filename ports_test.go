package dock_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	dock "github.com/usedock/dock-go"
)

const portJSON = `{"port":3000,"visibility":"private","url":"https://p.example","createdAt":"2026-01-01T00:00:00.000Z","extra":1}`

func TestPortsList(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":[`+portJSON+`,{"port":8080,"visibility":"public","url":null,"createdAt":"2026-01-01T00:00:00.000Z"}]}`)
	ps, err := c.Ports.List(context.Background(), "d 1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "GET" || got.Path != "/v1/docks/d 1/ports" {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
	if len(ps) != 2 || ps[0].Port != 3000 || ps[0].Visibility != dock.VisibilityPrivate || ps[0].URL == nil || *ps[0].URL != "https://p.example" {
		t.Fatalf("ports %+v", ps)
	}
	if ps[1].Visibility != dock.VisibilityPublic || ps[1].URL != nil {
		t.Fatalf("second port %+v", ps[1])
	}
}

func TestPortsExposeSendsVisibility(t *testing.T) {
	c, got := serve(t, 201, `{"ok":true,"data":{"port":3000,"visibility":"public","url":"https://p.example"}}`)
	p, err := c.Ports.Expose(context.Background(), "d1", 3000, dock.ExposePortParams{Visibility: dock.VisibilityPublic})
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "POST" || got.Path != "/v1/docks/d1/ports" || got.Body["port"] != float64(3000) || got.Body["visibility"] != "public" {
		t.Fatalf("%s %s %v", got.Method, got.Path, got.Body)
	}
	if p.Visibility != "public" {
		t.Fatalf("port %+v", p)
	}
}

func TestPortsExposeOmitsEmptyVisibility(t *testing.T) {
	c, got := serve(t, 201, `{"ok":true,"data":`+portJSON+`}`)
	if _, err := c.Ports.Expose(context.Background(), "d1", 3000, dock.ExposePortParams{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Body["visibility"]; ok || len(got.Body) != 1 {
		t.Fatalf("body %v", got.Body)
	}
}

func TestPortsRemove(t *testing.T) {
	c, got := serve(t, 200, `{"ok":true,"data":{"deleted":true}}`)
	if err := c.Ports.Remove(context.Background(), "d1", 3000); err != nil {
		t.Fatal(err)
	}
	if got.Method != "DELETE" || !strings.HasSuffix(got.Path, "/v1/docks/d1/ports/3000") {
		t.Fatalf("%s %s", got.Method, got.Path)
	}
}

func TestPortsRemoveNotExposed(t *testing.T) {
	c, _ := serve(t, 404, `{"ok":false,"code":"port_not_found","message":"Port 1 is not exposed"}`)
	err := c.Ports.Remove(context.Background(), "d1", 1)
	var apiErr *dock.Error
	if !errors.As(err, &apiErr) || apiErr.Code != "port_not_found" || apiErr.Status != 404 {
		t.Fatalf("err %v", err)
	}
}
