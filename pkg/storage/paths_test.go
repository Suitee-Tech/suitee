package storage

import "testing"

func TestExtractUploadObjectKey(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{
			name: "relative legacy path",
			raw:  "/uploads/toca-documents/tenant-1/tickets/ticket-1/Fundo.png",
			want: "tenant-1/tickets/ticket-1/Fundo.png",
			ok:   true,
		},
		{
			name: "absolute legacy URL",
			raw:  "https://api.example.com/uploads/toca-documents/tenant-1/tickets/ticket-1/Fundo.png?version=1",
			want: "tenant-1/tickets/ticket-1/Fundo.png",
			ok:   true,
		},
		{
			name: "encoded filename",
			raw:  "/uploads/toca-documents/tenant-1/tickets/ticket-1/arquivo%20final.png",
			want: "tenant-1/tickets/ticket-1/arquivo final.png",
			ok:   true,
		},
		{
			name: "different bucket",
			raw:  "/uploads/toca-avatars/tenant-1/avatar.png",
			ok:   false,
		},
		{
			name: "signed Supabase URL",
			raw:  "https://project.supabase.co/storage/v1/object/sign/toca-documents/tenant-1/file.png?token=abc",
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ExtractUploadObjectKey("toca-documents", tt.raw)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("ExtractUploadObjectKey() = (%q, %t), want (%q, %t)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
