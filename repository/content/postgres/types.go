package postgres

type (
	PrimaryKey struct {
		UserID string
		ID     string
		RefIDs []string
	}

	UpsertData struct {
		RefIDs      []string // only be used for update query
		PayloadJSON string
	}

	Response struct {
		UserID      string
		ID          string
		RefIDs      []string
		PayloadJSON string
	}
)
