package emailreview

import (
	"fmt"
	"strings"
	"testing"
)

func TestAddReviewBlocksPreview(t *testing.T) {
	input := `
	<html>
  <body>
    <table>
      <tr>
        <td>
          <img src="banner.png" alt="Your first sale is just the beginning">
        </td>
      </tr>
      <tr>
        <td>
          Hi Alex, congrats on your first sale. To unlock the next partner tier,
          complete the steps below and book a call with our team.
          <a href="https://example.com">Book a call</a>
        </td>
      </tr>
      <tr>
        <td>
          <a href="https://example.com/unsubscribe">Unsubscribe</a>
        </td>
      </tr>
    </table>
  </body>
</html>
	`

	result, err := AddReviewBlocks(input)
	if err != nil {
		t.Fatal(err)
	}

	fmt.Println("\n--- REVIEW HTML ---")
	fmt.Println(result)

	if !strings.Contains(result, `data-review-block="banner-001"`) {
		t.Fatal("expected banner review block")
	}

	if !strings.Contains(result, `data-review-block="body-001"`) {
		t.Fatal("expected body review block")
	}
}
