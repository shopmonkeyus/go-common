package logger

func KV(args ...interface{}) map[string]interface{} {
	m := make(map[string]interface{}, len(args)/2)
	for i := 0; i < len(args)-1; i += 2 {
		if key, ok := args[i].(string); ok {
			m[key] = args[i+1]
		}
	}
	return m
}
