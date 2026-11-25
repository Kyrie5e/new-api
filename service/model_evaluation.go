package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

type EvaluationQuestion struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Kind        string `json:"kind"`
	Version     string `json:"version"`
	MaxOutput   int    `json:"max_output"`
}

var EvaluationQuestions = []EvaluationQuestion{
	{ID: "logic", Name: "Logical reasoning", Description: "Solve a counting problem with changing parameters.", Kind: "text", Version: "logic-v1", MaxOutput: 4096},
	{ID: "instructions", Name: "Instruction following", Description: "Filter, sort and return exactly the requested JSON.", Kind: "text", Version: "instructions-v1", MaxOutput: 4096},
	{ID: "context", Name: "Context understanding", Description: "Follow revisions and extract the final facts.", Kind: "text", Version: "context-v1", MaxOutput: 4096},
	{ID: "animation", Name: "Animation creation", Description: "Create a standalone SVG and CSS animation for visual review.", Kind: "artwork", Version: "animation-v1", MaxOutput: 8192},
}

func BuildEvaluationQuestion(id string) (EvaluationQuestion, string, string, error) {
	var question EvaluationQuestion
	for _, candidate := range EvaluationQuestions {
		if candidate.ID == id {
			question = candidate
			break
		}
	}
	if question.ID == "" {
		return question, "", "", errors.New("Unknown evaluation question")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return question, "", "", err
	}
	variant := int(n.Int64())
	var prompt string
	var expected any
	switch id {
	case "logic":
		red, blue, green := 11+variant%7, 9+variant%5, 8+variant%3
		answer := max(blue+green+4, red+green+3)
		prompt = fmt.Sprintf("一个不透明袋子里有红球%d个、蓝球%d个、绿球%d个。球只能按颜色区分，摸取时不能识别颜色，每次不放回。最少摸出多少个球，才能无论摸取结果如何，都同时保证至少4个红球和至少3个蓝球？只输出一个JSON对象，格式为{\"answer\":整数}，不要解释、不要Markdown。", red, blue, green)
		expected = map[string]any{"answer": answer}
	case "instructions":
		base := 10 + variant%30
		prompt = fmt.Sprintf("按以下规则处理记录：仅保留enabled为true且score至少%d的记录；按score降序，分数相同按id字母升序；只返回JSON对象，且仅有ids和total两个字段，ids为有序id数组，total为保留记录的score之和。不要Markdown或解释。记录：[{\"id\":\"c\",\"score\":%d,\"enabled\":true},{\"id\":\"b\",\"score\":%d,\"enabled\":true},{\"id\":\"a\",\"score\":%d,\"enabled\":true},{\"id\":\"d\",\"score\":%d,\"enabled\":false},{\"id\":\"e\",\"score\":%d,\"enabled\":true}]", base+2, base+4, base+3, base+4, base+9, base+1)
		expected = map[string]any{"ids": []string{"a", "c", "b"}, "total": base*3 + 11}
	case "context":
		code := fmt.Sprintf("YU-%04d", variant)
		prompt = fmt.Sprintf("读取以下项目记录，只输出JSON对象，字段严格为code、owner、room、time，不要解释或Markdown。1. 项目%s最初负责人林安，会议室A201，时间周二09:00。2. 另一项目YU-9999负责人周宁，会议室C401，时间周四15:00，该项目不是本次查询目标。3. 目标项目负责人调整为陈雨，时间改为周三14:30，会议室暂时不变。4. 建议改在B302，但此建议被否决。5. 最终批准：目标项目会议室改为B305，时间保留上次已批准的安排，负责人不变。请返回目标项目最终信息。", code)
		expected = map[string]any{"code": code, "owner": "陈雨", "room": "B305", "time": "周三14:30"}
	case "animation":
		animals := []string{"企鹅", "水獭", "狐狸", "小熊"}
		vehicles := []string{"自行车", "三轮车", "滑板车"}
		prompt = fmt.Sprintf("创建一份可独立运行的完整HTML文档：一只%s骑着%s经过有树木、云朵和路灯的街道。用SVG和CSS实现精细插画及循环动画，包括角色动作、车轮旋转、背景移动。提供协调配色、完整构图和prefers-reduced-motion兼容。不得使用JavaScript、外部资源、网络请求、iframe、表单、图片链接或第三方库。只输出完整HTML，不要Markdown代码围栏。", animals[variant%len(animals)], vehicles[variant%len(vehicles)])
	}
	if expected == nil {
		return question, prompt, "", nil
	}
	encoded, err := common.Marshal(expected)
	return question, prompt, string(encoded), err
}

// Compare the entire JSON answer. Transport errors and truncation are separate.
func GradeEvaluationAnswer(answer, expected string) bool {
	var actualValue, expectedValue any
	if common.UnmarshalJsonStr(strings.TrimSpace(answer), &actualValue) != nil || common.UnmarshalJsonStr(expected, &expectedValue) != nil {
		return false
	}
	return reflect.DeepEqual(actualValue, expectedValue)
}

func EvaluationArtwork(answer string) string {
	html := strings.TrimSpace(answer)
	if strings.HasPrefix(html, "```") {
		if newline := strings.IndexByte(html, '\n'); newline >= 0 {
			html = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(html[newline+1:]), "```"))
		}
	}
	lower := strings.ToLower(html)
	if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") {
		return html
	}
	return ""
}
