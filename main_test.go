package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestCreateMarkdownParser_Footnotes(t *testing.T) {
	parser := createMarkdownParser()
	input := []byte("Text with reference.[^1]\n\n[^1]: This is the footnote.")
	var buf bytes.Buffer
	if err := parser.Convert(input, &buf); err != nil {
		t.Fatalf("Failed to parse markdown: %v", err)
	}

	output := buf.String()

	// Check if in-text footnote reference was parsed
	if !strings.Contains(output, `class="footnote-ref"`) {
		t.Errorf("Expected output to contain 'class=\"footnote-ref\"', got:\n%s", output)
	}

	// Check if footnotes container was parsed
	if !strings.Contains(output, `class="footnotes"`) {
		t.Errorf("Expected output to contain 'class=\"footnotes\"', got:\n%s", output)
	}

	// Check if footnote content is present
	if !strings.Contains(output, "This is the footnote.") {
		t.Errorf("Expected output to contain footnote text, got:\n%s", output)
	}
}
