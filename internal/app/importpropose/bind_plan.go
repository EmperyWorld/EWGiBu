package importpropose

// BindPlanItem describes one bind/build path the operator can stub or point at existing workspace content.
type BindPlanItem struct {
	ResourceID  string `json:"resourceId,omitempty"`
	ImageID     string `json:"imageId,omitempty"`
	MissingPath string `json:"missingPath"`
	Kind        string `json:"kind"` // directory | file | buildContext
	Source      string `json:"source,omitempty"`
	Include     string `json:"include,omitempty"`
	Status      string `json:"status"` // missing | exists
}
