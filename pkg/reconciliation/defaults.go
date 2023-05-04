package reconciliation

var (
	// Provides reasonable defaults for the logger container.
	DefaultsLoggerContainer = buildResourceRequirements(100, 64, 0, 128)

	// Provides reasonable defaults for the configuration container.
	DefaultsConfigInitContainer = buildResourceRequirements(1000, 256, 1000, 384)
)
