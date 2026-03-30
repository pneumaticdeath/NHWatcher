package screen

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// DarkTermTheme is a dark theme with a black background and light text,
// suitable for terminal/screensaver display. The TextScale field controls
// the monospace text size used by the terminal widget.
type DarkTermTheme struct {
	TextScale float32 // multiplier for text size (0 or 1 = default)
}

func (d *DarkTermTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return color.Black
	case theme.ColorNameForeground:
		return color.NRGBA{R: 204, G: 204, B: 204, A: 255}
	default:
		return theme.DefaultTheme().Color(name, theme.VariantDark)
	}
}

func (d *DarkTermTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (d *DarkTermTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (d *DarkTermTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNameText && d.TextScale > 0 && d.TextScale != 1 {
		return theme.DefaultTheme().Size(name) * d.TextScale
	}
	return theme.DefaultTheme().Size(name)
}
