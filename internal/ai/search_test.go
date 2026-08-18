package ai

import (
	"strings"
	"testing"
)

const bingFixture = `<html><body><ol id="b_results">
<li class="b_algo" data-id iid=SERP.5129><div class="b_tpcn"><a class="tilk">weather.com.cn</a></div>
<h2 class=""><a target="_blank" href="https://www.weather.com.cn/weather/101020100.shtml" h="ID=SERP,5129.2"><strong>上海</strong>天气预报_天气网</a></h2>
<div class="b_caption"><p class="b_lineclamp2">1 天前&ensp;&#0183;&ensp;上海天气预报，及时准确发布中央气象台天气信息，便捷查询上海今日天气。</p></div></li>
<li class="b_algo" data-id iid=SERP.5146>
<h2 class=""><a target="_blank" href="https://www.bing.com/ck/a?!&&u=a1aHR0cHM6Ly93d3cudGlhbnFpLmNvbS9zaGFuZ2hhaS8&amp;p=1" h="ID=SERP,5146.2">【上海天气】上海实时天气查询</a></h2>
</li>
<li class="b_algo" data-id iid=SERP.5160>
<h2 class=""><a target="_blank" href="javascript:void(0)">广告结果不应收录</a></h2>
</li>
<li class="b_pag">下一页</li>
</ol></body></html>`

func TestParseBingResults(t *testing.T) {
	results := parseBingResults(bingFixture, 5)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %+v", len(results), results)
	}
	if results[0].Title != "上海天气预报_天气网" {
		t.Errorf("title not cleaned: %q", results[0].Title)
	}
	if results[0].URL != "https://www.weather.com.cn/weather/101020100.shtml" {
		t.Errorf("unexpected url: %q", results[0].URL)
	}
	if !strings.Contains(results[0].Snippet, "上海天气预报") {
		t.Errorf("snippet not extracted: %q", results[0].Snippet)
	}
	if strings.Contains(results[0].Snippet, "&ensp;") || strings.Contains(results[0].Snippet, "&#0183;") {
		t.Errorf("html entities not decoded: %q", results[0].Snippet)
	}
	// 第二条是 bing /ck/a 跳转链接，应还原为真实 URL；javascript: 链接应被跳过
	if results[1].URL != "https://www.tianqi.com/shanghai/" {
		t.Errorf("redirect url not unwrapped: %q", results[1].URL)
	}
}

func TestUnwrapBingRedirectInvalidBase64(t *testing.T) {
	got := unwrapBingRedirect("https://www.bing.com/ck/a?u=a1!!!not-base64!!!")
	if got != "https://www.bing.com/ck/a?u=a1!!!not-base64!!!" {
		t.Errorf("invalid base64 should return original, got %q", got)
	}
	got = unwrapBingRedirect("https://example.com/page")
	if got != "https://example.com/page" {
		t.Errorf("non-bing url should stay unchanged, got %q", got)
	}
}

func TestCleanHTMLText(t *testing.T) {
	got := cleanHTMLText("  <strong>Hello</strong>&nbsp;World<br/> ")
	if got != "Hello World" {
		t.Errorf("cleanHTMLText = %q", got)
	}
}

func TestTruncateRunes(t *testing.T) {
	if got := truncateRunes("你好世界", 10); got != "你好世界" {
		t.Errorf("short string should stay: %q", got)
	}
	got := truncateRunes("你好世界你好世界", 4)
	if got != "你好世界…" {
		t.Errorf("truncateRunes = %q", got)
	}
}
