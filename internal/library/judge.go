package library

import (
	"errors"
	"fmt"
	"strings"
)

// JudgeTask is one "is this paper X?" question a model answers for many papers
// at once, and where an adopted "yes" lands. Screening is the task with an
// empty Question: the model judges against a person's requirement instead and
// an adopted answer becomes the paper's screening decision. Every other task
// marks the paper (an article type or a tag) only when the person adopts it.
type JudgeTask struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Question is what the model decides; empty for screening.
	Question string `json:"question"`
	// Basis says what the model should read: abstract (title, metadata and
	// abstract only, cheap and enough for most properties) or fulltext (the
	// stored full text when there is one, falling back to the abstract).
	Basis  string      `json:"basis"`
	Target JudgeTarget `json:"target"`
}

// JudgeTarget is where an adopted "yes" lands: the screening decision, an
// article type (Value is a canonical type such as review) or a personal tag
// (Value is the tag).
type JudgeTarget struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// JudgeTargetKinds a task may write to when adopted.
var JudgeTargetKinds = []string{"screening", "articleType", "tag"}

// JudgeBases a task may read.
var JudgeBases = []string{"abstract", "fulltext"}

// ScreeningTask is the built-in screening task; the requirement comes from the
// person at run time.
var ScreeningTask = JudgeTask{ID: "screening", Name: "收录筛选", Basis: "fulltext", Target: JudgeTarget{Kind: "screening"}}

// judgePresets are the built-in questions. Their ids are stable so a recorded
// suggestion still names its task after a preset is reworded.
var judgePresets = []JudgeTask{
	{
		ID: "review", Name: "是否综述", Basis: "abstract",
		Question: "这篇文献是否为综述（review）？综述指系统性或叙述性地总结已有研究、不报告新的原始实验数据的文章；方法学综述、perspective、commentary 也算综述。原创研究、资源论文、方法论文不算。",
		Target:   JudgeTarget{Kind: "articleType", Value: "review"},
	},
	{
		ID: "method", Name: "是否方法文章", Basis: "abstract",
		Question: "这篇文献是否主要提出、实现或系统评测一种新的计算方法、软件工具、实验方案或分析流程？只是应用已有方法做生物学研究的不算。",
		Target:   JudgeTarget{Kind: "articleType", Value: "method"},
	},
	{
		ID: "single-cell", Name: "是否单细胞测序", Basis: "abstract",
		Question: "这篇文献是否对样本做了单细胞或单核分辨率的测序（如 scRNA-seq、snRNA-seq、scATAC-seq、空间转录组等）并分析了这些数据？仅引用或讨论他人单细胞数据的不算。",
		Target:   JudgeTarget{Kind: "tag", Value: "单细胞"},
	},
	{
		ID: "public-data", Name: "是否公开原始数据", Basis: "fulltext",
		Question: "这篇文献是否公开了可下载的原始测序数据，并给出了数据库登录号（如 GEO、SRA、ENA、GSA、CNGB 等）？只写“数据可向作者索取”的不算。",
		Target:   JudgeTarget{Kind: "tag", Value: "公开数据"},
	},
}

// JudgePresets returns the built-in tasks, copied so callers cannot change them.
func JudgePresets() []JudgeTask {
	out := make([]JudgeTask, len(judgePresets))
	copy(out, judgePresets)
	return out
}

// JudgePreset finds a built-in task by id.
func JudgePreset(id string) (JudgeTask, bool) {
	for _, task := range judgePresets {
		if task.ID == id {
			return task, true
		}
	}
	return JudgeTask{}, false
}

// NormalizeJudgeTask trims a task and checks it can run and be adopted. A
// custom task needs a question and, for a tag target, the tag to write.
func NormalizeJudgeTask(task JudgeTask) (JudgeTask, error) {
	task.ID = strings.TrimSpace(task.ID)
	task.Name = strings.TrimSpace(task.Name)
	task.Question = strings.TrimSpace(task.Question)
	task.Basis = strings.ToLower(strings.TrimSpace(task.Basis))
	task.Target.Kind = strings.TrimSpace(task.Target.Kind)
	task.Target.Value = strings.TrimSpace(task.Target.Value)
	if task.Target.Kind == "" {
		task.Target.Kind = "tag"
	}
	if task.Basis == "" {
		task.Basis = "abstract"
	}
	if !oneOf(task.Basis, JudgeBases...) {
		return JudgeTask{}, fmt.Errorf("invalid judge basis %q", task.Basis)
	}
	switch task.Target.Kind {
	case "screening":
		task.Target.Value = ""
	case "articleType":
		canonical, ok := articleTypeLabels[task.Target.Value]
		if !ok {
			return JudgeTask{}, fmt.Errorf("invalid article type %q (use original, method, review, or other)", task.Target.Value)
		}
		task.Target.Value = canonical
		if task.Question == "" {
			return JudgeTask{}, errors.New("判断问题不能为空")
		}
	case "tag":
		if task.Target.Value == "" {
			return JudgeTask{}, errors.New("请填写采纳后要打的标签")
		}
		if task.Question == "" {
			return JudgeTask{}, errors.New("判断问题不能为空")
		}
	default:
		return JudgeTask{}, fmt.Errorf("invalid judge target %q", task.Target.Kind)
	}
	if task.ID == "" {
		task.ID = "custom"
	}
	if task.Name == "" {
		task.Name = task.Question
		if runes := []rune(task.Name); len(runes) > 40 {
			task.Name = string(runes[:40]) + "…"
		}
	}
	return task, nil
}
