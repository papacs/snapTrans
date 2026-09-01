package config

// Features only enable explicit user actions; experiments are off by default.
type Features struct {
	TextExtraction  bool `json:"textExtraction"`
	TableExtraction bool `json:"tableExtraction"`
	Pin             bool `json:"pin"`
	TextActions     bool `json:"textActions"`
	Redaction       bool `json:"redaction"`
	HistoryTools    bool `json:"historyTools"`
	MemeExplanation bool `json:"memeExplanation"`
	LearningCards   bool `json:"learningCards"`
	ShareCards      bool `json:"shareCards"`
	ImageCompare    bool `json:"imageCompare"`
}

// Pointer fields preserve the difference between an old config that did not
// know about a feature and a user explicitly disabling that feature.
type persistedFeatures struct {
	TextExtraction  *bool `json:"textExtraction"`
	TableExtraction *bool `json:"tableExtraction"`
	Pin             *bool `json:"pin"`
	TextActions     *bool `json:"textActions"`
	Redaction       *bool `json:"redaction"`
	HistoryTools    *bool `json:"historyTools"`
	MemeExplanation *bool `json:"memeExplanation"`
	LearningCards   *bool `json:"learningCards"`
	ShareCards      *bool `json:"shareCards"`
	ImageCompare    *bool `json:"imageCompare"`
}

func DefaultFeatures() Features {
	return Features{TextExtraction: true, TableExtraction: true, Pin: true, TextActions: true, Redaction: true, HistoryTools: true}
}

func loadedFeatures(saved *persistedFeatures) Features {
	features := DefaultFeatures()
	if saved == nil {
		return features
	}
	values := []struct {
		saved  *bool
		target *bool
	}{
		{saved.TextExtraction, &features.TextExtraction},
		{saved.TableExtraction, &features.TableExtraction},
		{saved.Pin, &features.Pin},
		{saved.TextActions, &features.TextActions},
		{saved.Redaction, &features.Redaction},
		{saved.HistoryTools, &features.HistoryTools},
		{saved.MemeExplanation, &features.MemeExplanation},
		{saved.LearningCards, &features.LearningCards},
		{saved.ShareCards, &features.ShareCards},
		{saved.ImageCompare, &features.ImageCompare},
	}
	for _, value := range values {
		if value.saved != nil {
			*value.target = *value.saved
		}
	}
	return features
}
