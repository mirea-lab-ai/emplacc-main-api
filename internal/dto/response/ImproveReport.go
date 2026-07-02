package response

type ImprovedReportResponse struct {
	TaskID          string `json:"task_id"`
	OriginalText    string `json:"original_text"`
	ImprovedText    string `json:"improved_text"`
	TaskTitle       string `json:"task_title,omitempty"`
	TaskDescription string `json:"task_description,omitempty"`
}
