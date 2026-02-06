package storage

type HistoryRecord struct {
	Date    string `json:"date"`
	Message string `json:"message"`
	Role    string `json:"role"`
}

func SaveHistory(repoName string, date string, message string, role string) error {
	return SaveHistoryDB(repoName, date, message, role)
}

func LoadHistory(repoName string, date string) (*HistoryRecord, error) {
	return LoadHistoryDB(repoName, date)
}
