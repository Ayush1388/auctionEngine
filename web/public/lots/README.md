# Lot photos (interim)

The API has no image storage yet, so lot photos are static files listed in
`manifest.json` in this folder. A lot is matched by the `Photos: <slug>` line
in its description header, or by its auction id.

```json
{
  "porsche-911s-1970": {
    "photos": [
      {
        "src": "/lots/porsche-911s-1970/lead-1600.jpg",
        "srcset": "/lots/porsche-911s-1970/lead-800.jpg 800w, /lots/porsche-911s-1970/lead-1600.jpg 1600w",
        "width": 1600,
        "height": 1067,
        "alt": "Orange 1970 Porsche 911 S, front three-quarter view",
        "color": "#8a6a4f"
      }
    ]
  }
}
```

The first photo is the lead image. Use a 3:2 crop with the car filling at
least 70% of the frame width. Lots without an entry show the typographic
placeholder (year and model).
