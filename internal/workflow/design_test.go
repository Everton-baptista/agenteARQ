package workflow

import (
	"math"
	"strings"
	"testing"
)

const goodTokens = `{
  "color": {
    "$type": "color",
    "ink": {"$value": {"colorSpace": "srgb", "components": [0.1, 0.1, 0.12], "hex": "#1a1a1f"}},
    "paper": {"$value": "#fbf8f2"},
    "accent": {"$value": {"colorSpace": "oklch", "components": [0.45, 0.12, 250]}},
    "text": {"$value": "{color.ink}"}
  },
  "font": {
    "body": {"$type": "fontFamily", "$value": ["Source Serif 4", "serif"],
      "$extensions": {"dev.agentarch": {"license": "OFL-1.1, Google Fonts"}}},
    "ui": {"$type": "fontFamily", "$value": "{font.body}"}
  },
  "space": {"$type": "dimension", "md": {"$value": {"value": 16, "unit": "px"}}},
  "$extensions": {"dev.agentarch": {"contrast": [
    {"fg": "{color.text}", "bg": "{color.paper}", "use": "text"},
    {"fg": "color.accent", "bg": "color.paper", "use": "ui"}
  ]}}
}`

func TestValidTokensHaveNoGaps(t *testing.T) {
	if gaps := TokenGaps([]byte(goodTokens)); len(gaps) != 0 {
		t.Fatalf("unexpected gaps: %v", gaps)
	}
}

func TestTokenGaps(t *testing.T) {
	cases := []struct {
		name, body string
		want       string
	}{
		{"not json", `[1,2]`, "is not a JSON object"},
		{"empty", `{}`, "defines no tokens"},
		{"cycle", `{"a": {"$type": "color", "$value": "{b}"}, "b": {"$value": "{a}"}}`, "alias cycle"},
		{"dangling alias", `{"a": {"$type": "color", "$value": "{nowhere}"}}`, "points to no token"},
		{"no type", `{"a": {"$value": 4}}`, "has no $type"},
		{"bad colour", `{"a": {"$type": "color", "$value": "blue"}}`, "is not a hex colour"},
		{"unsupported space", `{"a": {"$type": "color", "$value": {"colorSpace": "display-p3", "components": [1, 0, 0]}}}`, "is not checked yet"},
		{"font without licence", `{"f": {"$type": "fontFamily", "$value": "Inter"}}`, "must record its licence"},
		{"colours without pairs", `{"a": {"$type": "color", "$value": "#000000"}}`, "must declare the foreground/background pairs"},
		{"bad name", `{"a.b": {"$type": "color", "$value": "#000"}}`, "must not contain"},
		{"low contrast", `{"c": {"$type": "color", "grey": {"$value": "#999999"}, "white": {"$value": "#ffffff"}},
		  "$extensions": {"dev.agentarch": {"contrast": [{"fg": "c.grey", "bg": "c.white", "use": "text"}]}}}`, "needs at least 4.5:1"},
		{"translucent background", `{"c": {"$type": "color", "a": {"$value": "#000"}, "b": {"$value": "#ffffff80"}},
		  "$extensions": {"dev.agentarch": {"contrast": [{"fg": "c.a", "bg": "c.b", "use": "text"}]}}}`, "must be opaque"},
		{"pair on a non-colour", `{"c": {"$type": "color", "a": {"$value": "#000"}}, "f": {"$type": "fontFamily", "$value": "X", "$extensions": {"dev.agentarch": {"license": "OFL"}}},
		  "$extensions": {"dev.agentarch": {"contrast": [{"fg": "c.a", "bg": "f", "use": "ui"}]}}}`, "is not a colour token"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gaps := strings.Join(TokenGaps([]byte(c.body)), "\n")
			if !strings.Contains(gaps, c.want) {
				t.Fatalf("gaps %q do not mention %q", gaps, c.want)
			}
		})
	}
}

// Reference values: black on white is 21:1, #767676 on white is the classic 4.54:1 AA minimum.
func TestContrastRatioMatchesWCAG(t *testing.T) {
	white, _ := parseHex("#ffffff")
	for _, c := range []struct {
		hex  string
		want float64
	}{{"#000000", 21}, {"#767676", 4.54}, {"#777777", 4.48}} {
		fg, _ := parseHex(c.hex)
		if got := ContrastRatio(fg, white); math.Abs(got-c.want) > 0.01 {
			t.Errorf("%s on white = %.3f, want %.2f", c.hex, got, c.want)
		}
	}
	// OKLCH white and black convert to the sRGB extremes.
	r, g, b := oklchToLinearSRGB(1, 0, 0)
	if math.Abs(r-1) > 1e-3 || math.Abs(g-1) > 1e-3 || math.Abs(b-1) > 1e-3 {
		t.Fatalf("oklch white = %v %v %v", r, g, b)
	}
	r, g, b = oklchToLinearSRGB(0, 0, 0)
	if r+g+b > 1e-9 {
		t.Fatalf("oklch black = %v %v %v", r, g, b)
	}
}

func TestDesignBriefNeedsItsSections(t *testing.T) {
	s := specifiedFixture(t)
	write(t, s.Root, DesignBriefPath, "# Design brief\n\n## Público-alvo\n\nDonos de pequenas lojas, no celular, entre clientes.\n")
	write(t, s.Root, DesignTokensPath, goodTokens)
	_, e := s.Checkpoint(CheckpointRequest{Phase: "planned"})
	if e == nil {
		t.Fatal("an incomplete brief advanced to planned")
	}
	if !strings.Contains(e.Error(), `"## Subject"`) || !strings.Contains(e.Error(), `"## References"`) || strings.Contains(e.Error(), "Audience") {
		t.Fatalf("unexpected brief gaps: %v", e)
	}
	write(t, s.Root, DesignBriefPath, "# Design brief\n\n## Assunto\n\nUm balcão de pedidos para lojas de bairro.\n\n## Público-alvo\n\nDonos de pequenas lojas, no celular, entre clientes.\n\n## Referências\n\nCadernos de pedido de papel, etiquetas de feira.\n")
	if _, e := s.Checkpoint(CheckpointRequest{Phase: "planned"}); e != nil {
		t.Fatal(e)
	}
	n, _ := s.Next()
	if hasAdvice(n, AdviceDesign) {
		t.Fatal("design advice repeated after the brief and tokens exist")
	}
}

func TestBadTokensBlockPlanned(t *testing.T) {
	s := specifiedFixture(t)
	write(t, s.Root, DesignTokensPath, `{"f": {"$type": "fontFamily", "$value": "Inter"}}`)
	if _, e := s.Checkpoint(CheckpointRequest{Phase: "planned"}); e == nil || !strings.Contains(e.Error(), "licence") {
		t.Fatalf("want licence gap, got %v", e)
	}
}
