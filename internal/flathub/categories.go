package flathub

// Category is one hardcoded browse category (Bazaar-style).
// Hardcoded on purpose: Flathub occasionally adds categories and the TUI
// can't invent human labels/icons for unknown ones (see bazaar PR #1360).
// Slugs match the live `facetDistribution.main_categories` keys.
type Category struct {
	Slug  string // matches App.Categories, e.g. "Game"
	Label string // human label
	Icon  string // short glyph for the TUI
}

// Categories is the browse order shown on the home screen.
var Categories = []Category{
	{Slug: "All", Label: "All Apps", Icon: "*"},
	{Slug: "AudioVideo", Label: "Audio & Video", Icon: "~"},
	{Slug: "Development", Label: "Developer Tools", Icon: "{}"},
	{Slug: "Education", Label: "Education", Icon: "?"},
	{Slug: "Game", Label: "Games", Icon: ">"},
	{Slug: "Graphics", Label: "Graphics", Icon: "#"},
	{Slug: "Network", Label: "Internet", Icon: "@"},
	{Slug: "Office", Label: "Office & Productivity", Icon: "="},
	{Slug: "Science", Label: "Science", Icon: "+"},
	{Slug: "System", Label: "System", Icon: "%"},
	{Slug: "Utility", Label: "Utilities", Icon: "!"},
}

// LabelFor returns the human label for a raw AppStream category slug.
func LabelFor(slug string) string {
	for _, c := range Categories {
		if c.Slug == slug {
			return c.Label
		}
	}
	return slug
}
