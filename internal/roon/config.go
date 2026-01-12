package roon

type Config struct {
	// ExtensionID is a globally unique identifier for this extension.
	// Example: "com.yourname.roon-cover".
	ExtensionID string

	DisplayName    string
	DisplayVersion string
	Publisher      string
	Email          string
	Website        string
}
