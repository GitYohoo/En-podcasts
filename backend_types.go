package main

import (
	"path/filepath"
	"regexp"
	"strings"
)

var invalidBaseNameChars = regexp.MustCompile(`[<>:"/\\|?*]+`)
var whitespacePattern = regexp.MustCompile(`\s+`)

type OutputFiles struct {
	EnglishJSON    string `json:"englishJson"`
	EnglishTXT     string `json:"englishTxt"`
	EnglishSRT     string `json:"englishSrt"`
	ChineseJSON    string `json:"chineseJson"`
	ChineseTXT     string `json:"chineseTxt"`
	ReviewJSON     string `json:"reviewJson"`
	ReviewTXT      string `json:"reviewTxt"`
	ReviewManifest string `json:"reviewManifest"`
	ResultManifest string `json:"resultManifest"`
	OutputAudio    string `json:"outputAudio"`
}

type ReviewIssue struct {
	Category   string `json:"category"`
	Severity   string `json:"severity"`
	SourceText string `json:"sourceText"`
	Suggestion string `json:"suggestion"`
	Reason     string `json:"reason"`
}

type ReviewTurn struct {
	TurnIndex    int           `json:"turnIndex"`
	Speaker      string        `json:"speaker"`
	Start        float64       `json:"start"`
	End          float64       `json:"end"`
	StartTS      string        `json:"startTs"`
	EndTS        string        `json:"endTs"`
	OriginalText string        `json:"originalText"`
	ReviewedText string        `json:"reviewedText"`
	Issues       []ReviewIssue `json:"issues"`
}

type ReviewDraft struct {
	Summary    string       `json:"summary"`
	IssueCount int          `json:"issueCount"`
	Turns      []ReviewTurn `json:"turns"`
}

type ReviewManifest struct {
	InputAudio  string      `json:"inputAudio"`
	OutputDir   string      `json:"outputDir"`
	GeneratedAt string      `json:"generatedAt"`
	Turns       int         `json:"turns"`
	Issues      int         `json:"issues"`
	Summary     string      `json:"summary"`
	Manifest    string      `json:"manifest"`
	Files       OutputFiles `json:"files"`
}

type TranslationManifest struct {
	InputAudio  string      `json:"inputAudio"`
	OutputDir   string      `json:"outputDir"`
	GeneratedAt string      `json:"generatedAt"`
	Turns       int         `json:"turns"`
	Segments    int         `json:"segments"`
	Manifest    string      `json:"manifest"`
	Files       OutputFiles `json:"files"`
}

type SynthesisOptions struct {
	TranscriptPath           string  `json:"transcriptPath"`
	ReferenceAudioPath       string  `json:"referenceAudioPath"`
	OutputDir                string  `json:"outputDir"`
	OutputBaseName           string  `json:"outputBaseName"`
	Style                    string  `json:"style"`
	AddConversationMarkers   bool    `json:"addConversationMarkers"`
	PreserveTiming           bool    `json:"preserveTiming"`
	CoquiTOSAgreed           bool    `json:"coquiTOSAgreed"`
	Language                 string  `json:"language"`
	PauseMS                  int     `json:"pauseMs"`
	IntraTurnPauseMS         int     `json:"intraTurnPauseMs"`
	Speed                    float64 `json:"speed"`
	MaxCharsPerUtterance     int     `json:"maxCharsPerUtterance"`
	MaxSentencesPerUtterance int     `json:"maxSentencesPerUtterance"`
	FemaleSpeaker            string  `json:"femaleSpeaker"`
	MaleSpeaker              string  `json:"maleSpeaker"`
}

type RuntimeInfo struct {
	PythonExe         string `json:"pythonExe"`
	PythonPath        string `json:"pythonPath"`
	HFHome            string `json:"hfHome"`
	XTTSSite          string `json:"xttsSite"`
	XTTSSrc           string `json:"xttsSrc"`
	ExtraSitePackages string `json:"extraSitePackages"`
}

type ManifestRuntimeInfo struct {
	XTTSSite          string   `json:"xtts_site"`
	XTTSSrc           string   `json:"xtts_src"`
	ExtraSitePackages []string `json:"extra_site_packages"`
}

type ReferenceSegment struct {
	Start    float64 `json:"start"`
	End      float64 `json:"end"`
	Duration float64 `json:"duration"`
	Text     string  `json:"text"`
}

type SynthesisManifest struct {
	InputTranscript         string                        `json:"input_transcript"`
	OutputAudio             string                        `json:"output_audio"`
	GeneratedWith           string                        `json:"generated_with"`
	Style                   string                        `json:"style"`
	Device                  string                        `json:"device"`
	SampleRate              int                           `json:"sample_rate"`
	PauseMS                 int                           `json:"pause_ms"`
	IntraTurnPauseMS        int                           `json:"intra_turn_pause_ms"`
	PreserveTiming          bool                          `json:"preserve_timing"`
	AddConversationMarkers  bool                          `json:"add_conversation_markers"`
	Speed                   float64                       `json:"speed"`
	Speakers                map[string]string             `json:"speakers"`
	ReferenceAudio          string                        `json:"reference_audio"`
	SpeakerReferenceFiles   map[string][]string           `json:"speaker_reference_files"`
	SpeakerReferenceSegment map[string][]ReferenceSegment `json:"speaker_reference_segments"`
	AvailableSpeakersSample []string                      `json:"available_speakers_sample"`
	Turns                   int                           `json:"turns"`
	RuntimePaths            ManifestRuntimeInfo           `json:"runtime_paths"`
	ManifestPath            string                        `json:"manifest_path,omitempty"`
}

type JobState struct {
	Running               bool                `json:"running"`
	Stage                 string              `json:"stage"`
	Status                string              `json:"status"`
	Message               string              `json:"message"`
	Progress              float64             `json:"progress"`
	Error                 string              `json:"error"`
	AudioPath             string              `json:"audioPath"`
	ReferenceAudioPath    string              `json:"referenceAudioPath"`
	OutputDir             string              `json:"outputDir"`
	EnglishTranscriptPath string              `json:"englishTranscriptPath"`
	OutputAudioPath       string              `json:"outputAudioPath"`
	ManifestPath          string              `json:"manifestPath"`
	Logs                  []string            `json:"logs"`
	CommandPreview        string              `json:"commandPreview"`
	Files                 OutputFiles         `json:"files"`
	Review                ReviewDraft         `json:"review"`
	ReviewManifest        ReviewManifest      `json:"reviewManifest"`
	Translation           TranslationManifest `json:"translation"`
	Options               SynthesisOptions    `json:"options"`
	Runtime               RuntimeInfo         `json:"runtime"`
	Result                SynthesisManifest   `json:"result"`
}

func defaultOptions() SynthesisOptions {
	return SynthesisOptions{
		OutputBaseName:           defaultOutputBaseName,
		Style:                    "casual-podcast",
		AddConversationMarkers:   true,
		PreserveTiming:           true,
		CoquiTOSAgreed:           true,
		Language:                 "en",
		PauseMS:                  430,
		IntraTurnPauseMS:         160,
		Speed:                    0.98,
		MaxCharsPerUtterance:     125,
		MaxSentencesPerUtterance: 1,
	}
}

func normalizeOptions(options SynthesisOptions) SynthesisOptions {
	defaults := defaultOptions()
	if strings.TrimSpace(options.Style) == "" {
		options.Style = defaults.Style
	}
	if strings.TrimSpace(options.Language) == "" {
		options.Language = defaults.Language
	}
	if strings.TrimSpace(options.OutputBaseName) == "" {
		options.OutputBaseName = defaults.OutputBaseName
	}
	if options.PauseMS <= 0 {
		options.PauseMS = defaults.PauseMS
	}
	if options.IntraTurnPauseMS <= 0 {
		options.IntraTurnPauseMS = defaults.IntraTurnPauseMS
	}
	if options.Speed <= 0 {
		options.Speed = defaults.Speed
	}
	if options.MaxCharsPerUtterance <= 0 {
		options.MaxCharsPerUtterance = defaults.MaxCharsPerUtterance
	}
	if options.MaxSentencesPerUtterance <= 0 {
		options.MaxSentencesPerUtterance = defaults.MaxSentencesPerUtterance
	}
	options.TranscriptPath = cleanPath(options.TranscriptPath)
	options.ReferenceAudioPath = cleanPath(options.ReferenceAudioPath)
	if strings.TrimSpace(options.OutputDir) == "" && options.TranscriptPath != "" {
		options.OutputDir = filepath.Dir(options.TranscriptPath)
	} else {
		options.OutputDir = cleanPath(options.OutputDir)
	}
	options.OutputBaseName = sanitizeBaseName(options.OutputBaseName)
	options.FemaleSpeaker = strings.TrimSpace(options.FemaleSpeaker)
	options.MaleSpeaker = strings.TrimSpace(options.MaleSpeaker)
	return options
}

func sanitizeBaseName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultOutputBaseName
	}
	value = strings.TrimSuffix(value, filepath.Ext(value))
	value = invalidBaseNameChars.ReplaceAllString(value, "_")
	value = whitespacePattern.ReplaceAllString(value, "_")
	value = strings.Trim(value, "._ ")
	if value == "" {
		return defaultOutputBaseName
	}
	return value
}

func cloneState(state JobState) JobState {
	copied := state
	copied.Logs = append([]string(nil), state.Logs...)
	copied.Review.Turns = cloneReviewTurns(state.Review.Turns)
	copied.Result.AvailableSpeakersSample = append([]string(nil), state.Result.AvailableSpeakersSample...)
	copied.Result.RuntimePaths.ExtraSitePackages = append([]string(nil), state.Result.RuntimePaths.ExtraSitePackages...)
	if state.Result.Speakers != nil {
		copied.Result.Speakers = copyStringMap(state.Result.Speakers)
	}
	if state.Result.SpeakerReferenceFiles != nil {
		copied.Result.SpeakerReferenceFiles = copyStringSliceMap(state.Result.SpeakerReferenceFiles)
	}
	if state.Result.SpeakerReferenceSegment != nil {
		copied.Result.SpeakerReferenceSegment = copyReferenceSegmentMap(state.Result.SpeakerReferenceSegment)
	}
	return copied
}

func cloneReviewTurns(turns []ReviewTurn) []ReviewTurn {
	copied := make([]ReviewTurn, 0, len(turns))
	for _, turn := range turns {
		next := turn
		next.Issues = append([]ReviewIssue(nil), turn.Issues...)
		copied = append(copied, next)
	}
	return copied
}

func copyStringMap(source map[string]string) map[string]string {
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func copyStringSliceMap(source map[string][]string) map[string][]string {
	cloned := make(map[string][]string, len(source))
	for key, value := range source {
		cloned[key] = append([]string(nil), value...)
	}
	return cloned
}

func copyReferenceSegmentMap(source map[string][]ReferenceSegment) map[string][]ReferenceSegment {
	cloned := make(map[string][]ReferenceSegment, len(source))
	for key, value := range source {
		cloned[key] = append([]ReferenceSegment(nil), value...)
	}
	return cloned
}

func cleanPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return filepath.Clean(value)
}
