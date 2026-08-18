package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Tool 把一份暴露给模型的函数定义与本地执行逻辑绑定在一起。
type Tool struct {
	Spec ToolSpec
	Exec func(ctx context.Context, args string) (string, error)
}

// Registry 是当前 AI 好友可用的全部工具，新增工具往这里追加即可。
var Registry = []Tool{weatherTool, todayTool, webSearchTool}

// ToolSpecs 提取工具清单，用于随聊天请求发给模型。
func ToolSpecs(tools []Tool) []ToolSpec {
	specs := make([]ToolSpec, 0, len(tools))
	for _, t := range tools {
		specs = append(specs, t.Spec)
	}
	return specs
}

// ExecTool 在注册表中查找并执行一次工具调用。任何失败都返回一段可作为
// role=tool 消息回传给模型的 JSON 错误文本，让模型自行向用户解释，
// 而不是让整次回复失败。
func ExecTool(ctx context.Context, call ToolCall) string {
	for _, t := range Registry {
		if t.Spec.Function.Name != call.Function.Name {
			continue
		}
		result, err := t.Exec(ctx, call.Function.Arguments)
		if err != nil {
			return toolErrorJSON(err)
		}
		return result
	}
	return toolErrorJSON(fmt.Errorf("unknown tool %q", call.Function.Name))
}

func toolErrorJSON(err error) string {
	data, err := json.Marshal(map[string]string{"error": err.Error()})
	if err != nil {
		return `{"error":"tool failed"}`
	}
	return string(data)
}

// ---- 日期工具：模型没有内置的"今天"概念，日期时间问题必须查本工具 ----

var todayTool = Tool{
	Spec: ToolSpec{
		Type: "function",
		Function: FunctionDef{
			Name:        "get_today",
			Description: "获取今天的日期、星期几和当前时间。用户询问今天几号、星期几、现在几点、今年是哪年等日期时间问题时必须调用本工具，不要凭记忆猜测。",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	},
	Exec: execToday,
}

func execToday(ctx context.Context, argsJSON string) (string, error) {
	return buildTodayInfo(time.Now()), nil
}

var weekdayText = [...]string{"星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"}

func buildTodayInfo(now time.Time) string {
	data, err := json.Marshal(map[string]interface{}{
		"date":    now.Format("2006-01-02"),
		"weekday": weekdayText[now.Weekday()],
		"time":    now.Format("15:04"),
		"year":    now.Year(),
		"month":   int(now.Month()),
		"day":     now.Day(),
	})
	if err != nil {
		return toolErrorJSON(err)
	}
	return string(data)
}

// ---- 天气工具：数据源为 Open-Meteo，免密钥，支持中文城市名 ----

const (
	geocodingBaseURL = "https://geocoding-api.open-meteo.com/v1/search"
	forecastBaseURL  = "https://api.open-meteo.com/v1/forecast"
)

var weatherHTTP = &http.Client{Timeout: 15 * time.Second}

var weatherTool = Tool{
	Spec: ToolSpec{
		Type: "function",
		Function: FunctionDef{
			Name:        "get_weather",
			Description: "查询指定城市当前的实时天气，包括气温、体感温度、天气状况、湿度、风速。用户询问任何地点的天气时都必须调用本工具获取真实数据，不要编造。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"city": map[string]interface{}{
						"type":        "string",
						"description": "城市名称，支持中文，例如：上海、北京、Tokyo",
					},
				},
				"required": []string{"city"},
			},
		},
	},
	Exec: execWeather,
}

func execWeather(ctx context.Context, argsJSON string) (string, error) {
	var args struct {
		City string `json:"city"`
	}
	if err := json.Unmarshal([]byte(argsJSON), &args); err != nil {
		return "", fmt.Errorf("arguments is not valid json: %w", err)
	}
	city := strings.TrimSpace(args.City)
	if city == "" {
		return "", errors.New("city is required")
	}

	location, err := geocode(ctx, city)
	if err != nil {
		return "", err
	}
	weather, err := fetchCurrentWeather(ctx, location.Latitude, location.Longitude)
	if err != nil {
		return "", err
	}

	result, err := json.Marshal(map[string]interface{}{
		"city":        location.DisplayName(),
		"temperature": fmt.Sprintf("%.0f°C", weather.Temperature),
		"feels_like":  fmt.Sprintf("%.0f°C", weather.ApparentTemperature),
		"condition":   describeWeatherCode(weather.Code),
		"humidity":    fmt.Sprintf("%d%%", weather.Humidity),
		"wind_speed":  fmt.Sprintf("%.0f km/h", weather.WindSpeed),
	})
	if err != nil {
		return "", err
	}
	return string(result), nil
}

type geoLocation struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Country   string  `json:"country"`
	Admin1    string  `json:"admin1"`
}

// DisplayName 拼接去重后的地名，例如 "上海, 中国"。
func (g geoLocation) DisplayName() string {
	parts := make([]string, 0, 3)
	for _, p := range []string{g.Name, g.Admin1, g.Country} {
		p = strings.TrimSpace(p)
		if p != "" && !containsStr(parts, p) {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func containsStr(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

func geocode(ctx context.Context, city string) (*geoLocation, error) {
	query := url.Values{}
	query.Set("name", city)
	query.Set("count", "1")
	query.Set("language", "zh")
	query.Set("format", "json")

	var resp struct {
		Results []geoLocation `json:"results"`
	}
	if err := getJSON(ctx, geocodingBaseURL+"?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("geocode %q: %w", city, err)
	}
	if len(resp.Results) == 0 {
		return nil, fmt.Errorf("未找到城市 %q，请换一个更常见的名称", city)
	}
	return &resp.Results[0], nil
}

type currentWeather struct {
	Temperature         float64 `json:"temperature_2m"`
	ApparentTemperature float64 `json:"apparent_temperature"`
	Humidity            int     `json:"relative_humidity_2m"`
	Code                int     `json:"weather_code"`
	WindSpeed           float64 `json:"wind_speed_10m"`
}

func fetchCurrentWeather(ctx context.Context, latitude, longitude float64) (*currentWeather, error) {
	query := url.Values{}
	query.Set("latitude", fmt.Sprintf("%.4f", latitude))
	query.Set("longitude", fmt.Sprintf("%.4f", longitude))
	query.Set("current", "temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m")
	query.Set("timezone", "auto")

	var resp struct {
		Current currentWeather `json:"current"`
	}
	if err := getJSON(ctx, forecastBaseURL+"?"+query.Encode(), &resp); err != nil {
		return nil, fmt.Errorf("fetch weather: %w", err)
	}
	return &resp.Current, nil
}

// weatherCodeText 是 Open-Meteo 使用的 WMO 天气代码到中文描述的映射。
var weatherCodeText = map[int]string{
	0: "晴", 1: "大致晴朗", 2: "局部多云", 3: "阴",
	45: "雾", 48: "雾凇",
	51: "小毛毛雨", 53: "毛毛雨", 55: "大毛毛雨",
	56: "冻毛毛雨", 57: "强冻毛毛雨",
	61: "小雨", 63: "中雨", 65: "大雨",
	66: "冻雨", 67: "强冻雨",
	71: "小雪", 73: "中雪", 75: "大雪", 77: "雪粒",
	80: "小阵雨", 81: "阵雨", 82: "强阵雨",
	85: "小阵雪", 86: "阵雪",
	95: "雷暴", 96: "雷暴伴冰雹", 99: "强雷暴伴冰雹",
}

func describeWeatherCode(code int) string {
	if text, ok := weatherCodeText[code]; ok {
		return text
	}
	return fmt.Sprintf("未知天气代码 %d", code)
}

func getJSON(ctx context.Context, rawURL string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := weatherHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	return json.Unmarshal(data, out)
}
