package config

// Features only enable explicit user actions; experiments are off by default.
type Features struct {
	TextExtraction  bool `json:"textExtraction"`
	Pin             bool `json:"pin"`
	TextActions     bool `json:"textActions"`
	Redaction       bool `json:"redaction"`
	HistoryTools    bool `json:"historyTools"`
	MemeExplanation bool `json:"memeExplanation"`
	LearningCards   bool `json:"learningCards"`
	ShareCards      bool `json:"shareCards"`
	ImageCompare    bool `json:"imageCompare"`
}

func DefaultFeatures() Features {
	return Features{TextExtraction: true, Pin: true, TextActions: true, Redaction: true, HistoryTools: true}
}
func loadedFeatures(saved *Features) Features {
	if saved == nil {
		return DefaultFeatures()
	}
	return *saved
}
