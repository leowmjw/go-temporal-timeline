package hcl

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
)

const (
	// ContentTypeHCL is the custom MIME type for HCL configuration
	ContentTypeHCL = "application/vnd.hcl"
	
	// ContentTypeJSON is the standard MIME type for JSON
	ContentTypeJSON = "application/json"
)

// DetectContentType determines if the content is JSON or HCL based on content-type header and content inspection
func DetectContentType(r *http.Request) (string, error) {
	// First, check if Content-Type header is present and valid
	contentType := r.Header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			// Check for recognized media types
			if mediaType == ContentTypeHCL {
				return ContentTypeHCL, nil
			}
			if mediaType == ContentTypeJSON {
				return ContentTypeJSON, nil
			}
		}
	}

	// If Content-Type is not set or not recognized, inspect the content
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read request body: %w", err)
	}
	
	// Reset the body so it can be read again later
	r.Body = io.NopCloser(bytes.NewBuffer(body))
	
	// Trim whitespace to make detection more reliable
	trimmedBody := bytes.TrimSpace(body)
	
	// Simple heuristic: JSON starts with { or [, HCL typically doesn't
	if len(trimmedBody) > 0 {
		firstChar := trimmedBody[0]
		if firstChar == '{' || firstChar == '[' {
			// Looks like JSON
			return ContentTypeJSON, nil
		}
		
		// If it has '=' or '{' after identifier, likely HCL
		if IsHCL(trimmedBody) {
			return ContentTypeHCL, nil
		}
	}
	
	// Default to JSON if we can't determine
	return ContentTypeJSON, nil
}

// IsHCLBasedOnExtension checks if the filename has an HCL extension
func IsHCLBasedOnExtension(filename string) bool {
	return strings.HasSuffix(filename, ".hcl") || 
	       strings.HasSuffix(filename, ".tf") ||
	       strings.HasSuffix(filename, ".tfvars")
}
