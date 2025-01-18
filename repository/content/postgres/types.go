package postgres

type (
	PrimaryKey struct {
		Namespace string
		RefIDs    []string
		ID        string
	}

	UpsertData struct {
		Data []byte
		Meta []byte
	}
)
