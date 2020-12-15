package tracing

// Values converts user data to arpc.Message's values
func Values(value interface{}) map[interface{}]interface{} {
	return map[interface{}]interface{}{
		appenderName: value,
	}
}
