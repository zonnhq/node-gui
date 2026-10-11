package client

import "testing"

func TestTarget(t *testing.T) {
	cases := []struct {
		in, target string
		tls        bool
	}{
		{"localhost:8080", "localhost:8080", false},
		{"http://10.0.0.5:8080/", "10.0.0.5:8080", false},
		{"https://nodes.zonnhq.com", "nodes.zonnhq.com:443", true},
		{"https://nodes.zonnhq.com/", "nodes.zonnhq.com:443", true},
		{"https://nodes.localhost:8443", "nodes.localhost:8443", true},
		{" 7.tcp.eu.ngrok.io:12345 ", "7.tcp.eu.ngrok.io:12345", false},
	}
	for _, c := range cases {
		target, tls := Target(c.in)
		if target != c.target || tls != c.tls {
			t.Errorf("Target(%q) = %q, %v; want %q, %v", c.in, target, tls, c.target, c.tls)
		}
	}
	if Host("https://nodes.zonnhq.com") != "nodes.zonnhq.com" || Host("localhost:8080") != "localhost" {
		t.Fatal("Host did not strip the scheme and port")
	}
}
