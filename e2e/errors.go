package e2e

// extractErrorMessage returns the error message or the empty string if nil
func extractErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
