package split

import (
	"testing"
	"time"
)

func TestTexts(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 11, d, 10, 0, 0, 0, time.UTC) }
	text := func(id string, first, last int) Text {
		return Text{GroupID: id, FirstSentAt: day(first), LastSentAt: day(last), Sends: 1}
	}
	ids := func(texts []Text) []string {
		out := []string{}
		for _, text := range texts {
			out = append(out, text.GroupID)
		}
		return out
	}

	for _, tc := range []struct {
		name  string
		texts []Text
		want  []string
	}{
		{"one text", []Text{text("a", 1, 9)}, nil},
		{"two robots, two texts", []Text{text("old", 1, 9), text("new", 3, 8)}, []string{"new", "old"}},
		{"old text stopped before the new one started", []Text{text("old", 1, 3), text("new", 4, 9)}, nil},
		{"old text trails off within a day", []Text{text("old", 1, 4), text("new", 4, 9)}, nil},
		{"old text keeps going", []Text{text("old", 1, 6), text("new", 4, 9)}, []string{"new", "old"}},
		{"three texts, one replaced", []Text{text("a", 1, 2), text("b", 3, 9), text("c", 5, 9)}, []string{"c", "b"}},
		{"new text started today: too early to tell", []Text{text("old", 1, 9), text("new", 9, 9)}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Texts(tc.texts)
			if tc.want == nil {
				if got != nil {
					t.Fatalf("got %v, want not split", ids(got))
				}
				return
			}
			if g, w := ids(got), tc.want; len(g) != len(w) || g[0] != w[0] || g[1] != w[1] {
				t.Fatalf("got %v, want %v", g, w)
			}
		})
	}
}

func TestFromJSON(t *testing.T) {
	texts, err := FromJSON([]byte(`[
		{"group_id": "1", "first_sent_at": "2026-10-01T09:00:00+00:00", "last_sent_at": "2026-11-09T09:00:00.5+00:00", "sends": 4},
		{"group_id": "2", "first_sent_at": "2026-11-02T09:00:00+00:00", "last_sent_at": "2026-11-09T10:00:00+00:00", "sends": 7}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 2 || texts[0].GroupID != "2" || texts[0].Sends != 7 {
		t.Fatalf("got %+v", texts)
	}
	if texts, err := FromJSON(nil); err != nil || texts != nil {
		t.Fatalf("got %v, %v for no texts", texts, err)
	}
}
