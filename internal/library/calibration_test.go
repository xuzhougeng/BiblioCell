package library

import (
	"reflect"
	"testing"
)

func pair(id, paper, profile, model, run, verdict, decision, at string) CalibrationPair {
	kind := "screen"
	if run != "" {
		kind = "batch"
	}
	return CalibrationPair{SuggestionID: id, PaperID: paper, Title: "Paper " + paper, ProfileID: profile, ProfileName: "Profile " + profile,
		Model: model, RunID: run, Kind: kind, Verdict: verdict, Decision: decision, CreatedAt: at}
}

func cell(report CalibrationReport, verdict, decision string) int {
	for _, c := range report.Matrix {
		if c.Verdict == verdict && c.Decision == decision {
			return c.Count
		}
	}
	return -1
}

func TestCalibrateAgreementAndMatrix(t *testing.T) {
	pairs := []CalibrationPair{
		pair("s6", "p6", "a", "m1", "", "uncertain", "", "2026-01-06T00:00:00Z"), // still pending
		pair("s5", "p5", "a", "m1", "", "excluded", "included", "2026-01-05T00:00:00Z"),
		pair("s4", "p4", "a", "m1", "", "uncertain", "uncertain", "2026-01-04T00:00:00Z"),
		pair("s3", "p3", "a", "m1", "", "included", "excluded", "2026-01-03T00:00:00Z"),
		pair("s2", "p2", "a", "m1", "", "excluded", "excluded", "2026-01-02T00:00:00Z"),
		pair("s1", "p1", "a", "m1", "", "included", "included", "2026-01-01T00:00:00Z"),
	}
	report := Calibrate(pairs, CalibrationFilter{})
	if report.Total != 6 || report.Compared != 5 || report.Pending != 1 || report.Agreed != 3 {
		t.Fatalf("counts = %+v", report)
	}
	if report.Rate != 0.6 {
		t.Fatalf("rate = %v", report.Rate)
	}
	if !reflect.DeepEqual(report.Verdicts, []string{"included", "excluded", "uncertain"}) || len(report.Matrix) != 9 {
		t.Fatalf("matrix axis = %v, cells = %d", report.Verdicts, len(report.Matrix))
	}
	for _, want := range []struct {
		verdict, decision string
		count             int
	}{{"included", "included", 1}, {"included", "excluded", 1}, {"excluded", "excluded", 1}, {"excluded", "included", 1}, {"uncertain", "uncertain", 1}, {"uncertain", "included", 0}, {"excluded", "uncertain", 0}} {
		if got := cell(report, want.verdict, want.decision); got != want.count {
			t.Errorf("matrix[%s][%s] = %d, want %d", want.verdict, want.decision, got, want.count)
		}
	}
	// Mismatches are the compared pairs that differ, newest first.
	ids := []string{}
	for _, m := range report.Mismatches {
		ids = append(ids, m.SuggestionID)
	}
	if !reflect.DeepEqual(ids, []string{"s5", "s3"}) {
		t.Fatalf("mismatches = %v", ids)
	}
	if len(report.ByModel) != 1 || report.ByModel[0].Compared != 5 || report.ByModel[0].Agreed != 3 || report.ByModel[0].Rate != 0.6 {
		t.Fatalf("by model = %+v", report.ByModel)
	}
}

func TestCalibrateEmpty(t *testing.T) {
	report := Calibrate(nil, CalibrationFilter{})
	if report.Total != 0 || report.Rate != 0 || len(report.Matrix) != 9 || report.Mismatches == nil || report.Models == nil || report.Runs == nil {
		t.Fatalf("empty report = %+v", report)
	}
}

func TestCalibrateKeepsNewestPerPaperAndModel(t *testing.T) {
	pairs := []CalibrationPair{
		// The same model answered p1 twice: only the newer answer counts.
		pair("s3", "p1", "a", "m1", "", "excluded", "included", "2026-02-01T00:00:00Z"),
		pair("s1", "p1", "a", "m1", "", "included", "included", "2026-01-01T00:00:00Z"),
		// A different model answered the same paper: counted on its own.
		pair("s2", "p1", "b", "m2", "", "included", "included", "2026-01-15T00:00:00Z"),
	}
	report := Calibrate(pairs, CalibrationFilter{})
	if report.Total != 2 || report.Compared != 2 || report.Agreed != 1 {
		t.Fatalf("deduped counts = %+v", report)
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].SuggestionID != "s3" {
		t.Fatalf("mismatches = %+v", report.Mismatches)
	}
	if len(report.ByModel) != 2 || report.ByModel[0].ProfileID != "a" || report.ByModel[0].Agreed != 0 || report.ByModel[1].ProfileID != "b" || report.ByModel[1].Agreed != 1 {
		t.Fatalf("by model = %+v", report.ByModel)
	}
	// Order of input does not matter: the newest still wins.
	reversed := []CalibrationPair{pairs[1], pairs[2], pairs[0]}
	if again := Calibrate(reversed, CalibrationFilter{}); again.Agreed != 1 || again.Mismatches[0].SuggestionID != "s3" {
		t.Fatalf("order-dependent result: %+v", again)
	}
}

func TestCalibrateFilters(t *testing.T) {
	pairs := []CalibrationPair{
		pair("s4", "p2", "b", "m2", "run2", "excluded", "included", "2026-03-02T10:00:00Z"),
		pair("s3", "p1", "b", "m2", "run2", "included", "included", "2026-03-02T09:00:00Z"),
		pair("s2", "p2", "a", "m1", "run1", "included", "included", "2026-03-01T10:00:00Z"),
		pair("s1", "p1", "a", "m1", "run1", "included", "included", "2026-03-01T09:00:00Z"),
		pair("s0", "p1", "a", "m1", "", "excluded", "included", "2026-02-01T09:00:00Z"),
	}
	byProfile := Calibrate(pairs, CalibrationFilter{ProfileID: "a"})
	if byProfile.Total != 2 || byProfile.Agreed != 2 {
		t.Fatalf("profile filter = %+v", byProfile)
	}
	byModel := Calibrate(pairs, CalibrationFilter{Model: "m2"})
	if byModel.Total != 2 || byModel.Agreed != 1 {
		t.Fatalf("model filter = %+v", byModel)
	}
	byRun := Calibrate(pairs, CalibrationFilter{RunID: "run1"})
	if byRun.Total != 2 || byRun.Agreed != 2 || len(byRun.ByModel) != 1 {
		t.Fatalf("run filter = %+v", byRun)
	}
	byDate := Calibrate(pairs, CalibrationFilter{From: "2026-03-02", To: "2026-03-02"})
	if byDate.Total != 2 || byDate.Agreed != 1 {
		t.Fatalf("date filter = %+v", byDate)
	}
	before := Calibrate(pairs, CalibrationFilter{To: "2026-02-28"})
	if before.Total != 1 || before.Agreed != 0 {
		t.Fatalf("to filter = %+v", before)
	}
	// Filter menus still list everything.
	if len(byRun.Models) != 2 || len(byRun.Runs) != 2 {
		t.Fatalf("menus = %+v / %+v", byRun.Models, byRun.Runs)
	}
	if byRun.Runs[0].RunID != "run2" || byRun.Runs[0].StartedAt != "2026-03-02T09:00:00Z" || byRun.Runs[0].Total != 2 || byRun.Runs[0].Agreed != 1 || byRun.Runs[0].Rate != 0.5 {
		t.Fatalf("runs = %+v", byRun.Runs)
	}
	if byRun.Runs[1].RunID != "run1" || byRun.Runs[1].Rate != 1 || byRun.Runs[1].ProfileName != "Profile a" {
		t.Fatalf("runs = %+v", byRun.Runs)
	}
	// The unfiltered model list dedupes like the report: profile a answered p1 twice.
	if byRun.Models[0].ProfileID != "a" || byRun.Models[0].Compared != 2 || byRun.Models[0].Agreed != 2 {
		t.Fatalf("models = %+v", byRun.Models)
	}
}

func TestLocatePage(t *testing.T) {
	pages := []string{"Title page\nAbstract: roots were\nsequenced.", "", "Methods: Libraries were prepared\nwith 10x Genomics Chromium.\nData under GSE123."}
	for _, c := range []struct {
		excerpt string
		page    int
	}{
		{"Libraries were prepared with 10x Genomics Chromium.", 3},
		{"roots were sequenced", 1},
		{"", 0},
		{"nowhere to be found", 0},
		// A long quote the model shortened or that spans a break is placed by its opening words.
		{"Libraries were prepared with 10x Genomics Chromium. Data under GSE123. Then something the model invented at the end which is long enough to matter here", 3},
	} {
		if got := LocatePage(pages, c.excerpt); got != c.page {
			t.Errorf("LocatePage(%q) = %d, want %d", c.excerpt, got, c.page)
		}
	}
	if PageLocation(0) != "" || PageLocation(4) != "page 4" || PageOf("page 4") != 4 || PageOf("Methods, page 3") != 0 || PageOf("") != 0 {
		t.Fatal("page location round trip failed")
	}
}
