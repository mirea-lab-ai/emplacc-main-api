package request

type ImproveReportRequest struct {
	UserText string `json:"user_text" validate:"required,min=1,max=2000"`
}
