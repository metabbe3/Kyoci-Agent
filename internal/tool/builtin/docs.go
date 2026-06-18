package builtin

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/metabbe3/Kyoci-Agent/pkg"
)

// ==============================================================================================
// DocsTool — Context7-style on-demand documentation lookup
// ==============================================================================================
// This tool fetches up-to-date documentation and best practices for frontend
// libraries and web technologies. It follows the 8B design principle: instead
// of hoping the model remembers APIs, it fetches fresh docs on demand.
//
// Strategy:
// 1. Try Context7 API (https://context7.com/api) for the library
// 2. Fall back to DevDocs (https://devdocs.io) which has curated docs
// 3. Fall back to fetching the official docs page directly

// DocsTool fetches documentation and best practices for libraries/frameworks.
type DocsTool struct {
	logger *slog.Logger
	client *http.Client
}

// NewDocsTool creates a new docs tool instance.
func NewDocsTool() *DocsTool {
	return &DocsTool{
		logger: slog.Default(),
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Name returns the tool name.
func (d *DocsTool) Name() string {
	return "docs"
}

// Description returns the tool description.
func (d *DocsTool) Description() string {
	return "Fetch up-to-date documentation and best practices for a library, framework, or web technology. " +
		"Use this BEFORE writing code when you need current API details, examples, or best practices. " +
		"Examples: library='react' topic='hooks', library='tailwindcss' topic='flexbox', library='next.js' topic='routing'."
}

// Parameters returns the tool parameter definition.
func (d *DocsTool) Parameters() []kyoci.ToolParameter {
	return []kyoci.ToolParameter{
		{
			Name:        "library",
			Type:        "string",
			Description: "The library or technology name (e.g. 'react', 'tailwindcss', 'next.js', 'typescript', 'css', 'html', 'node', 'express')",
			Required:    true,
		},
		{
			Name:        "topic",
			Type:        "string",
			Description: "Specific topic or API within the library (e.g. 'hooks', 'grid', 'routing', 'generics', 'middleware')",
			Required:    false,
		},
	}
}

// Execute fetches documentation for the requested library and topic.
func (d *DocsTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	library, ok := params["library"].(string)
	if !ok || library == "" {
		return "", fmt.Errorf("library parameter is required")
	}

	topic, _ := params["topic"].(string)

	library = strings.ToLower(strings.TrimSpace(library))
	topic = strings.ToLower(strings.TrimSpace(topic))

	// Try multiple documentation sources in order
	result, err := d.fetchDocs(ctx, library, topic)
	if err != nil {
		return "", fmt.Errorf("failed to fetch docs for %s: %w", library, err)
	}

	// Truncate to keep context manageable for 8B models
	if len(result) > 4000 {
		result = result[:4000] + "\n\n[... documentation truncated, use web_search or browser fetch for more]"
	}

	return result, nil
}

// fetchDocs tries multiple documentation sources.
func (d *DocsTool) fetchDocs(ctx context.Context, library, topic string) (string, error) {
	// Build a curated URL based on the library
	docURL := d.resolveDocURL(library, topic)
	if docURL == "" {
		// Unknown library — use a general search approach
		return d.searchGeneralDocs(ctx, library, topic)
	}

	d.logger.Info("fetching documentation", "library", library, "topic", topic, "url", docURL)

	// Fetch the documentation page
	content, err := d.fetchPage(ctx, docURL)
	if err != nil {
		d.logger.Warn("failed to fetch curated docs, trying search", "url", docURL, "error", err)
		return d.searchGeneralDocs(ctx, library, topic)
	}

	// Format the result
	header := fmt.Sprintf("=== %s Documentation", strings.Title(library))
	if topic != "" {
		header += fmt.Sprintf(" — %s ===", strings.Title(topic))
	} else {
		header += " ==="
	}

	return header + "\n\n" + content, nil
}

// resolveDocURL maps a library name + topic to the best documentation URL.
func (d *DocsTool) resolveDocURL(library, topic string) string {
	type docSource struct {
		baseURL string
		paths   map[string]string // topic -> path
	}

	// Curated documentation URLs for popular libraries
	sources := map[string]docSource{
		"react": {
			baseURL: "https://react.dev/reference",
			paths: map[string]string{
				"hooks":     "https://react.dev/reference/react/hooks",
				"usestate":  "https://react.dev/reference/react/useState",
				"useeffect": "https://react.dev/reference/react/useEffect",
				"component": "https://react.dev/reference/react/Component",
				"context":   "https://react.dev/reference/react/useContext",
				"refs":      "https://react.dev/reference/react/useRef",
				"rendering": "https://react.dev/learn/render-and-commit",
				"state":     "https://react.dev/learn/state-a-components-memory",
			},
		},
		"next.js": {
			baseURL: "https://nextjs.org/docs",
			paths: map[string]string{
				"routing":    "https://nextjs.org/docs/app/building-your-application/routing",
				"api":        "https://nextjs.org/docs/app/building-your-application/routing/route-handlers",
				"layouts":    "https://nextjs.org/docs/app/building-your-application/routing/layouts-and-templates",
				"middleware": "https://nextjs.org/docs/app/building-your-application/routing/middleware",
				"images":     "https://nextjs.org/docs/app/building-your-application/optimizing/images",
				"fonts":      "https://nextjs.org/docs/app/building-your-application/optimizing/fonts",
			},
		},
		"nextjs": {
			baseURL: "https://nextjs.org/docs",
			paths: map[string]string{
				"routing": "https://nextjs.org/docs/app/building-your-application/routing",
			},
		},
		"tailwindcss": {
			baseURL: "https://tailwindcss.com/docs",
			paths: map[string]string{
				"flexbox":    "https://tailwindcss.com/docs/flex",
				"grid":       "https://tailwindcss.com/docs/grid-template-columns",
				"spacing":    "https://tailwindcss.com/docs/padding",
				"colors":     "https://tailwindcss.com/docs/customizing-colors",
				"responsive": "https://tailwindcss.com/docs/responsive-design",
				"dark":       "https://tailwindcss.com/docs/dark-mode",
				"animation":  "https://tailwindcss.com/docs/animation",
			},
		},
		"tailwind": {
			baseURL: "https://tailwindcss.com/docs",
			paths: map[string]string{
				"flexbox": "https://tailwindcss.com/docs/flex",
				"grid":    "https://tailwindcss.com/docs/grid-template-columns",
			},
		},
		"typescript": {
			baseURL: "https://www.typescriptlang.org/docs",
			paths: map[string]string{
				"generics":  "https://www.typescriptlang.org/docs/handbook/2/generics.html",
				"types":     "https://www.typescriptlang.org/docs/handbook/2/everyday-types.html",
				"narrowing": "https://www.typescriptlang.org/docs/handbook/2/narrowing.html",
				"functions": "https://www.typescriptlang.org/docs/handbook/2/functions.html",
				"classes":   "https://www.typescriptlang.org/docs/handbook/2/classes.html",
				"modules":   "https://www.typescriptlang.org/docs/handbook/2/modules.html",
				"utility":   "https://www.typescriptlang.org/docs/handbook/utility-types.html",
			},
		},
		"css": {
			baseURL: "https://developer.mozilla.org/en-US/docs/Web/CSS",
			paths: map[string]string{
				"flexbox":   "https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_flexible_box_layout",
				"grid":      "https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_grid_layout",
				"variables": "https://developer.mozilla.org/en-US/docs/Web/CSS/--*",
				"animation": "https://developer.mozilla.org/en-US/docs/Web/CSS/animation",
				"selectors": "https://developer.mozilla.org/en-US/docs/Web/CSS/CSS_selectors",
				"position":  "https://developer.mozilla.org/en-US/docs/Web/CSS/position",
			},
		},
		"html": {
			baseURL: "https://developer.mozilla.org/en-US/docs/Web/HTML",
			paths: map[string]string{
				"forms":         "https://developer.mozilla.org/en-US/docs/Web/HTML/Element/form",
				"accessibility": "https://developer.mozilla.org/en-US/docs/Web/Accessibility",
				"semantic":      "https://developer.mozilla.org/en-US/docs/Glossary/Semantics",
			},
		},
		"node": {
			baseURL: "https://nodejs.org/docs/latest/api/",
			paths: map[string]string{
				"fs":            "https://nodejs.org/docs/latest/api/fs.html",
				"path":          "https://nodejs.org/docs/latest/api/path.html",
				"http":          "https://nodejs.org/docs/latest/api/http.html",
				"stream":        "https://nodejs.org/docs/latest/api/stream.html",
				"events":        "https://nodejs.org/docs/latest/api/events.html",
				"child_process": "https://nodejs.org/docs/latest/api/child_process.html",
			},
		},
		"node.js": {
			baseURL: "https://nodejs.org/docs/latest/api/",
			paths: map[string]string{
				"fs": "https://nodejs.org/docs/latest/api/fs.html",
			},
		},
		"express": {
			baseURL: "https://expressjs.com/en/4x/api.html",
			paths: map[string]string{
				"routing":    "https://expressjs.com/en/guide/routing.html",
				"middleware": "https://expressjs.com/en/guide/using-middleware.html",
				"static":     "https://expressjs.com/en/4x/api.html#express.static",
				"error":      "https://expressjs.com/en/guide/error-handling.html",
			},
		},
		"vue": {
			baseURL: "https://vuejs.org/guide/introduction.html",
			paths: map[string]string{
				"components":  "https://vuejs.org/guide/essentials/component-basics.html",
				"reactivity":  "https://vuejs.org/guide/essentials/reactivity-fundamentals.html",
				"composables": "https://vuejs.org/guide/reusability/composables.html",
			},
		},
		"svelte": {
			baseURL: "https://svelte.dev/docs",
			paths: map[string]string{
				"components": "https://svelte.dev/docs/svelte-components",
				"runes":      "https://svelte.dev/docs/svelte/$state",
			},
		},
		"astro": {
			baseURL: "https://docs.astro.build",
			paths: map[string]string{
				"routing":    "https://docs.astro.build/en/guides/routing/",
				"components": "https://docs.astro.build/en/basics/astro-components/",
			},
		},
		"alpine": {
			baseURL: "https://alpinejs.dev",
			paths: map[string]string{
				"directives": "https://alpinejs.dev/directives/data",
				"state":      "https://alpinejs.dev/directives/data",
			},
		},
		"alpine.js": {
			baseURL: "https://alpinejs.dev",
			paths: map[string]string{
				"directives": "https://alpinejs.dev/directives/data",
			},
		},
	}

	source, exists := sources[library]
	if !exists {
		return ""
	}

	// If we have a topic-specific URL, use it
	if topic != "" {
		// Normalize topic (remove spaces)
		topicKey := strings.ReplaceAll(topic, " ", "")
		if path, ok := source.paths[topicKey]; ok {
			return path
		}
	}

	return source.baseURL
}

// searchGeneralDocs falls back to a web search for unknown libraries.
func (d *DocsTool) searchGeneralDocs(ctx context.Context, library, topic string) (string, error) {
	query := library
	if topic != "" {
		query = library + " " + topic
	}
	query += " documentation best practices"

	// Use DuckDuckGo's instant answer API
	searchURL := fmt.Sprintf("https://lite.duckduckgo.com/lite/?q=%s", url.QueryEscape(query))
	content, err := d.fetchPage(ctx, searchURL)
	if err != nil {
		return fmt.Sprintf("Could not fetch docs for '%s'. Try using web_search tool with query: %s", library, query), nil
	}

	header := fmt.Sprintf("=== Search results for: %s ===\n\n", query)
	return header + content, nil
}

// fetchPage retrieves a web page and extracts clean text.
func (d *DocsTool) fetchPage(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, pageURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return cleanHTML(string(body)), nil
}

// cleanHTML strips HTML tags and returns readable text.
var (
	// RE2 doesn't support backreferences, so we match common block elements separately
	scriptRe     = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe      = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	navRe        = regexp.MustCompile(`(?is)<nav[^>]*>.*?</nav>`)
	headerRe     = regexp.MustCompile(`(?is)<header[^>]*>.*?</header>`)
	footerRe     = regexp.MustCompile(`(?is)<footer[^>]*>.*?</footer>`)
	svgRe        = regexp.MustCompile(`(?is)<svg[^>]*>.*?</svg>`)
	htmlTagRe    = regexp.MustCompile(`<[^>]+>`)
	whitespaceRe = regexp.MustCompile(`\n{3,}`)
	entityRe     = regexp.MustCompile(`&[a-z]+;|&#\d+;`)
)

func cleanHTML(html string) string {
	// Remove script, style, nav, header, footer, svg blocks
	text := scriptRe.ReplaceAllString(html, "")
	text = styleRe.ReplaceAllString(text, "")
	text = navRe.ReplaceAllString(text, "")
	text = headerRe.ReplaceAllString(text, "")
	text = footerRe.ReplaceAllString(text, "")
	text = svgRe.ReplaceAllString(text, "")
	// Remove code blocks content that's too verbose (keep inline code)
	// Remove all HTML tags
	text = htmlTagRe.ReplaceAllString(text, "\n")
	// Decode common entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = entityRe.ReplaceAllString(text, "")
	// Collapse excessive whitespace
	text = whitespaceRe.ReplaceAllString(text, "\n\n")
	// Trim each line
	lines := strings.Split(text, "\n")
	var cleanLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleanLines = append(cleanLines, trimmed)
		}
	}
	result := strings.Join(cleanLines, "\n")
	// Final cleanup
	if len(result) > 4000 {
		result = result[:4000]
	}
	return result
}
