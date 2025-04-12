package locale

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"go.uber.org/zap"
	"golang.org/x/text/language"
)

func init() {
	caddy.RegisterModule(Locale{})
	httpcaddyfile.RegisterHandlerDirective("locale", parseCaddyfile)
}

// Locale implements an HTTP handler that provides locale functionality
type Locale struct {
	// AvailableLocales contains the list of supported locales
	AvailableLocales []string `json:"available_locales,omitempty"`

	// Matcher is used to match the best available language
	matcher language.Matcher

	logger *zap.Logger
}

// CaddyModule returns the Caddy module information.
func (Locale) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.locale",
		New: func() caddy.Module { return new(Locale) },
	}
}

// Provision sets up the module.
func (m *Locale) Provision(ctx caddy.Context) error {
	m.logger = ctx.Logger(m)

	// Convert available locales to language tags
	var tags []language.Tag
	for _, loc := range m.AvailableLocales {
		// Try to parse as a simple language code first
		tag := language.Make(loc)
		if tag == language.Und {
			// If that fails, try parsing as a full language tag
			var err error
			tag, err = language.Parse(loc)
			if err != nil {
				return fmt.Errorf("invalid locale %q: %v", loc, err)
			}
		}
		// Create a clean tag for consistency
		cleanTag := makeCleanTag(tag)
		tags = append(tags, cleanTag)
	}

	// Create a matcher for the available languages
	m.matcher = language.NewMatcher(tags)

	// Log the available tags
	m.logger.Debug("available locales",
		zap.Strings("input", m.AvailableLocales),
		zap.Strings("tags", func() []string {
			result := make([]string, len(tags))
			for i, tag := range tags {
				result[i] = tag.String()
			}
			return result
		}()))

	return nil
}

// Validate implements caddy.Validator.
func (m *Locale) Validate() error {
	if len(m.AvailableLocales) == 0 {
		return fmt.Errorf("no available locales specified")
	}
	return nil
}

// makeCleanTag creates a clean language tag from the given tag, preserving script and region when present
func makeCleanTag(tag language.Tag) language.Tag {
	// Get the language components
	base, _ := tag.Base()
	script, scriptConf := tag.Script()
	region, regionConf := tag.Region()

	// Create a clean language tag
	if scriptConf == language.Exact && regionConf == language.Exact {
		// If we have both script and region, use them
		return language.Make(base.String() + "-" + script.String() + "-" + region.String())
	} else if scriptConf == language.Exact {
		// If we only have script, use it
		return language.Make(base.String() + "-" + script.String())
	} else if regionConf == language.Exact {
		// If we only have region, use it
		return language.Make(base.String() + "-" + region.String())
	}
	// Otherwise just use the base language
	return language.Make(base.String())
}

// ServeHTTP implements caddyhttp.MiddlewareHandler.
func (m Locale) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// Get Accept-Language header
	acceptLang := r.Header.Get("Accept-Language")

	// Parse the preferred languages - the matcher will automatically handle fallbacks
	tag, confidence := language.MatchStrings(m.matcher, acceptLang)

	// Create a clean language tag
	cleanTag := makeCleanTag(tag)

	// Log the language matching details
	m.logger.Debug("language matching",
		zap.String("accept_language", acceptLang),
		zap.String("best_match", cleanTag.String()),
		zap.Int("confidence", int(confidence)))

	// Store the selected locale in the request context in lowercase
	ctx := context.WithValue(r.Context(), caddy.ReplacerCtxKey, caddy.NewReplacer())
	repl := ctx.Value(caddy.ReplacerCtxKey).(*caddy.Replacer)
	locale := strings.ToLower(cleanTag.String())
	repl.Set("locale", locale)

	// Log the final locale being used
	m.logger.Debug("selected locale",
		zap.String("locale", locale))

	// Create a new request with the context
	r = r.WithContext(ctx)

	return next.ServeHTTP(w, r)
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (m *Locale) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		// Parse available_locales argument
		if !d.NextArg() {
			return d.ArgErr()
		}

		// Split the locales by comma
		locales := strings.Split(d.Val(), ",")
		for i := range locales {
			locales[i] = strings.TrimSpace(locales[i])
		}
		m.AvailableLocales = locales

		if d.NextArg() {
			return d.ArgErr()
		}
	}
	return nil
}

// parseCaddyfile unmarshals tokens from h into a new Middleware.
func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m Locale
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return m, err
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Locale)(nil)
	_ caddy.Validator             = (*Locale)(nil)
	_ caddyhttp.MiddlewareHandler = (*Locale)(nil)
	_ caddyfile.Unmarshaler       = (*Locale)(nil)
)
