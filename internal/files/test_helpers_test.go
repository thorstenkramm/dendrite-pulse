package files

func defaultOptions() Options {
	return Options{
		MaxUploadBytes: 2 * 1024 * 1024 * 1024,
		FileMode:       0o600,
	}
}
