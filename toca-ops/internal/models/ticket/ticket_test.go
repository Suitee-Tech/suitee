package ticket

import "testing"

func TestComment_EffectiveVisibility(t *testing.T) {
	tests := []struct {
		name string
		c    Comment
		want CommentVisibility
	}{
		{
			name: "explicit public",
			c:    Comment{Visibility: CommentVisibilityPublic},
			want: CommentVisibilityPublic,
		},
		{
			name: "explicit internal",
			c:    Comment{Visibility: CommentVisibilityInternal},
			want: CommentVisibilityInternal,
		},
		{
			name: "empty falls back to public (legacy backward-compat)",
			c:    Comment{},
			want: CommentVisibilityPublic,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.EffectiveVisibility(); got != tt.want {
				t.Errorf("EffectiveVisibility() = %v, want %v", got, tt.want)
			}
			wantPublic := tt.want == CommentVisibilityPublic
			if got := tt.c.IsPublic(); got != wantPublic {
				t.Errorf("IsPublic() = %v, want %v", got, wantPublic)
			}
		})
	}
}

func TestCommentVisibility_IsValid(t *testing.T) {
	tests := []struct {
		v    CommentVisibility
		want bool
	}{
		{CommentVisibilityPublic, true},
		{CommentVisibilityInternal, true},
		{"", false},
		{"unknown", false},
		{"PUBLIC", false},
	}
	for _, tt := range tests {
		if got := tt.v.IsValid(); got != tt.want {
			t.Errorf("CommentVisibility(%q).IsValid() = %v, want %v", tt.v, got, tt.want)
		}
	}
}

func TestNewCommentWithVisibility_InvalidFallsBackToPublic(t *testing.T) {
	c := NewCommentWithVisibility("author", "Name", "operator", "content", nil, "garbage")
	if c.Visibility != CommentVisibilityPublic {
		t.Errorf("invalid visibility should fall back to public, got %q", c.Visibility)
	}
}

func TestFilterPublicComments(t *testing.T) {
	comments := []Comment{
		{ID: "1", Visibility: CommentVisibilityPublic, Content: "Olá cliente"},
		{ID: "2", Visibility: CommentVisibilityInternal, Content: "Aguardar jurídico antes de responder"},
		{ID: "3", Visibility: "", Content: "Legacy comment without visibility"},
		{ID: "4", Visibility: CommentVisibilityPublic, Content: "Resolvido"},
	}
	got := FilterPublicComments(comments)
	if len(got) != 3 {
		t.Fatalf("expected 3 public comments (including legacy), got %d", len(got))
	}
	wantIDs := map[string]bool{"1": true, "3": true, "4": true}
	for _, c := range got {
		if !wantIDs[c.ID] {
			t.Errorf("unexpected comment in output: id=%q visibility=%q", c.ID, c.Visibility)
		}
	}
	for _, c := range got {
		if c.ID == "2" {
			t.Fatal("internal comment leaked into public filter — PRIVACY ISSUE")
		}
	}
}

func TestFilterPublicComments_EmptyInput(t *testing.T) {
	got := FilterPublicComments(nil)
	if len(got) != 0 {
		t.Errorf("nil input should return empty slice, got len=%d", len(got))
	}
	got = FilterPublicComments([]Comment{})
	if len(got) != 0 {
		t.Errorf("empty input should return empty slice, got len=%d", len(got))
	}
}
