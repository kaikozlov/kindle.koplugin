package jsonout

type ScanBook struct {
	ID          string   `json:"id"`
	SourcePath  string   `json:"source_path"`
	SidecarPath string   `json:"sidecar_path,omitempty"`
	Format      string   `json:"format"`
	LogicalExt  string   `json:"logical_ext"`
	Title       string   `json:"title"`
	Authors     []string `json:"authors"`
	DisplayName string   `json:"display_name"`
	OpenMode    string   `json:"open_mode"`
	BlockReason string   `json:"block_reason,omitempty"`
	SourceMtime int64    `json:"source_mtime"`
	SourceSize  int64    `json:"source_size"`
}

type ScanResult struct {
	Version int        `json:"version"`
	Root    string     `json:"root"`
	Books   []ScanBook `json:"books"`
}

type ConvertResult struct {
	Version    int    `json:"version"`
	OK         bool   `json:"ok"`
	Code       string `json:"code,omitempty"`
	Message    string `json:"message,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
}
