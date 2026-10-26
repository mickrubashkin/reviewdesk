package emailtext

import "testing"

func TestExtractContentPartsDoesNotInferFooterLinkAsPrimaryCTA(t *testing.T) {
	parts := ExtractContentParts(`
		<html>
			<body>
				<p>Thanks for joining.</p>
				<a href="https://www.bitrix24.com/privacy/">Privacy policy</a>
			</body>
		</html>
	`, "", "", "")

	if parts.PrimaryCTA != "" {
		t.Fatalf("expected empty primary CTA, got %q", parts.PrimaryCTA)
	}
	if len(parts.LinkGroups.Primary) != 0 {
		t.Fatalf("expected no primary links, got %#v", parts.LinkGroups.Primary)
	}
	if len(parts.LinkGroups.Footer) != 1 || parts.LinkGroups.Footer[0] != "Privacy policy" {
		t.Fatalf("expected privacy link in footer group, got %#v", parts.LinkGroups.Footer)
	}
}

func TestExtractContentPartsUsesStyledNonFooterLinkAsPrimaryCTA(t *testing.T) {
	parts := ExtractContentParts(`
		<html>
			<body>
				<a href="https://example.com/start" style="display: inline-block; border-radius: 6px">Start setup</a>
				<a href="https://www.bitrix24.com/privacy/">Privacy policy</a>
			</body>
		</html>
	`, "", "", "")

	if parts.PrimaryCTA != "Start setup" {
		t.Fatalf("expected styled link as primary CTA, got %q", parts.PrimaryCTA)
	}
	if len(parts.LinkGroups.Primary) != 1 || parts.LinkGroups.Primary[0] != "Start setup" {
		t.Fatalf("expected start link in primary group, got %#v", parts.LinkGroups.Primary)
	}
	if len(parts.LinkGroups.Footer) != 1 || parts.LinkGroups.Footer[0] != "Privacy policy" {
		t.Fatalf("expected privacy link in footer group, got %#v", parts.LinkGroups.Footer)
	}
}
