package storage

type HistoryRecord struct {
	Date   string `json:"date"`
	Report string `json:"report,omitempty"`
}

func SaveHistory(repoName string, date string, report string) error {
	return SaveHistoryDB(repoName, date, report)
}

func LoadHistory(repoName string, date string) (*HistoryRecord, error) {
	return LoadHistoryDB(repoName, date)
}
