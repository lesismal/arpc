package tracing

// Values converts user data to arpc.Message's values
func Values(value interface{}) map[string]interface{} {
	return map[string]interface{}{
		appenderName: value,
	}
}
