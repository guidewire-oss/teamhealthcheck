package main

import "testing"

func TestDirectoryRedirectPath(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		isDir  bool
		want   string
		wantOK bool
	}{
		{name: "directory without slash", path: "/docs/api", isDir: true, want: "/docs/api/", wantOK: true},
		{name: "directory with slash", path: "/docs/api/", isDir: true},
		{name: "file", path: "/docs/assets/main.css", isDir: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := directoryRedirectPath(tt.path, tt.isDir)
			if got != tt.want || ok != tt.wantOK {
				t.Fatalf("directoryRedirectPath(%q, %t) = (%q, %t), want (%q, %t)", tt.path, tt.isDir, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
