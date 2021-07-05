package reconciliation

var (
	// Provides reasonable defaults for the logger container.
	DefaultsLoggerContainer = buildResourceRequirements(100, 64)

	// Provides reasonable defaults for the configuration container.
	DefaultsConfigInitContainer = buildResourceRequirements(1000, 256)
)
