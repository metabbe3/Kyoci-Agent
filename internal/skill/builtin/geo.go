package builtin

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Geography skills — great-circle distance, lat/lon validation/parsing, country
// code conversions (ISO 3166-1), currency code lookup (ISO 4217), currency symbols.
//
// All data is baked in — no network, no external APIs.
// =====================================================================================

// EarthRadiusKm is the mean Earth radius in kilometres (WGS-84 volumetric).
const EarthRadiusKm = 6371.0

// ---- haversine_distance ----

type HaversineDistanceSkill struct{ *kyoci.BaseSkill }

func NewHaversineDistanceSkill() *HaversineDistanceSkill {
	return &HaversineDistanceSkill{BaseSkill: kyoci.NewBaseSkill(
		"haversine_distance",
		"Great-circle distance between two lat,lon points (km). "+
			"Usage: 'haversine: 37.7749,-122.4194 to 40.7128,-74.0060'",
		[]string{"haversine", "haversine distance", "great circle distance"},
	)}
}

func (s *HaversineDistanceSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "haversine") || strings.Contains(low, "great circle distance")
}

func (s *HaversineDistanceSkill) Execute(_ context.Context, q string) (string, error) {
	// stripVerb is used because the operand contains commas/colons which
	// extractPayload would mangle.
	operand := stripVerb(q, "haversine distance")
	if strings.EqualFold(strings.TrimSpace(operand), strings.TrimSpace(q)) {
		// "haversine distance" verb not present — fall back to plain "haversine".
		operand = stripVerb(q, "haversine")
	}
	operand = strings.TrimPrefix(strings.TrimSpace(operand), ":")
	operand = strings.TrimSpace(operand)
	operand = strings.TrimSuffix(operand, "km")
	operand = strings.TrimSpace(operand)
	if operand == "" {
		return "", fmt.Errorf("expected 'haversine: <lat1,lon1> to <lat2,lon2>'")
	}
	// Split the two coordinate pairs on " to ".
	sep := " to "
	idx := strings.Index(strings.ToLower(operand), sep)
	if idx < 0 {
		return "", fmt.Errorf("expected ' to ' separating the two points")
	}
	left := strings.TrimSpace(operand[:idx])
	right := strings.TrimSpace(operand[idx+len(sep):])
	lat1, lon1, err := parseLatLon(left)
	if err != nil {
		return "", fmt.Errorf("first point: %w", err)
	}
	lat2, lon2, err := parseLatLon(right)
	if err != nil {
		return "", fmt.Errorf("second point: %w", err)
	}
	d := haversineKm(lat1, lon1, lat2, lon2)
	bearing := bearingDeg(lat1, lon1, lat2, lon2)
	return fmt.Sprintf("distance: %.2f km\nbearing: %.1f°", d, bearing), nil
}

// parseLatLon parses a "lat,lon" pair, allowing surrounding whitespace and an
// optional separating space after the comma.
func parseLatLon(s string) (lat, lon float64, err error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, ",", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected 'lat,lon' pair, got %q", s)
	}
	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude %q: %w", parts[0], err)
	}
	lon, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude %q: %w", parts[1], err)
	}
	return lat, lon, nil
}

// haversineKm computes the great-circle distance between two points on Earth.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180.0
	φ1 := lat1 * rad
	φ2 := lat2 * rad
	dφ := (lat2 - lat1) * rad
	dλ := (lon2 - lon1) * rad
	a := math.Sin(dφ/2)*math.Sin(dφ/2) +
		math.Cos(φ1)*math.Cos(φ2)*math.Sin(dλ/2)*math.Sin(dλ/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return EarthRadiusKm * c
}

// bearingDeg returns the initial bearing (forward azimuth) in degrees.
func bearingDeg(lat1, lon1, lat2, lon2 float64) float64 {
	rad := math.Pi / 180.0
	φ1 := lat1 * rad
	φ2 := lat2 * rad
	dλ := (lon2 - lon1) * rad
	y := math.Sin(dλ) * math.Cos(φ2)
	x := math.Cos(φ1)*math.Sin(φ2) - math.Sin(φ1)*math.Cos(φ2)*math.Cos(dλ)
	θ := math.Atan2(y, x)
	b := math.Mod(θ/rad+360, 360)
	return b
}

// ---- latlon_validate ----

type LatlonValidateSkill struct{ *kyoci.BaseSkill }

func NewLatlonValidateSkill() *LatlonValidateSkill {
	return &LatlonValidateSkill{BaseSkill: kyoci.NewBaseSkill(
		"latlon_validate",
		"Validate a 'lat,lon' pair (lat -90..90, lon -180..180). "+
			"Usage: 'latlon validate: 37.7749, -122.4194'",
		[]string{"latlon validate", "latitude longitude validate", "validate latlon"},
	)}
}

func (s *LatlonValidateSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "latlon validate") ||
		strings.Contains(low, "lat lon validate") ||
		strings.Contains(low, "latitude longitude validate") ||
		strings.Contains(low, "validate latlon") ||
		strings.Contains(low, "validate lat lon")
}

func (s *LatlonValidateSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		// Fall back to stripping the verb (handles "latlon validate:" with
		// no payload, or oddly-formatted queries).
		payload = stripVerb(q, "latlon validate")
	}
	payload = strings.TrimSpace(payload)
	lat, lon, err := parseLatLon(payload)
	if err != nil {
		return fmt.Sprintf("invalid: %v", err), nil
	}
	if lat < -90 || lat > 90 {
		return fmt.Sprintf("invalid: latitude %g out of range [-90, 90]", lat), nil
	}
	if lon < -180 || lon > 180 {
		return fmt.Sprintf("invalid: longitude %g out of range [-180, 180]", lon), nil
	}
	return fmt.Sprintf("valid: lat=%g lon=%g", lat, lon), nil
}

// ---- latlon_parse ----

type LatlonParseSkill struct{ *kyoci.BaseSkill }

func NewLatlonParseSkill() *LatlonParseSkill {
	return &LatlonParseSkill{BaseSkill: kyoci.NewBaseSkill(
		"latlon_parse",
		"Parse a 'lat,lon' pair into normalized form 'lat=X lon=Y'. "+
			"Usage: 'latlon parse: 37.7749, -122.4194'",
		[]string{"latlon parse", "parse latlon", "parse lat lon"},
	)}
}

func (s *LatlonParseSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "latlon parse") ||
		strings.Contains(low, "parse latlon") ||
		strings.Contains(low, "parse lat lon") ||
		strings.Contains(low, "parse latitude longitude")
}

func (s *LatlonParseSkill) Execute(_ context.Context, q string) (string, error) {
	payload := extractPayload(q)
	if payload == "" {
		payload = stripVerb(q, "latlon parse")
	}
	payload = strings.TrimSpace(payload)
	lat, lon, err := parseLatLon(payload)
	if err != nil {
		return "", fmt.Errorf("invalid lat,lon: %w", err)
	}
	// Normalize: drop unnecessary precision, then reformat.
	return fmt.Sprintf("lat=%s lon=%s",
		strconv.FormatFloat(lat, 'g', -1, 64),
		strconv.FormatFloat(lon, 'g', -1, 64)), nil
}

// =====================================================================================
// ISO 3166-1 country tables.
//
// Single source of truth: isoAlpha2ToCountry maps ISO 3166-1 alpha-2 →
// {Alpha3, Name}. We derive alpha3→alpha2 and name→alpha2 from this table at
// init time so there's no duplication and no risk of drift.
// =====================================================================================

type isoCountry struct {
	Alpha3 string
	Name   string
}

// isoAlpha2ToCountry is the canonical country table. ~150 entries — covers
// every UN member state plus the most commonly referenced territories.
var isoAlpha2ToCountry = map[string]isoCountry{
	"AD": {"AND", "Andorra"},
	"AE": {"ARE", "United Arab Emirates"},
	"AF": {"AFG", "Afghanistan"},
	"AG": {"ATG", "Antigua and Barbuda"},
	"AI": {"AIA", "Anguilla"},
	"AL": {"ALB", "Albania"},
	"AM": {"ARM", "Armenia"},
	"AO": {"AGO", "Angola"},
	"AQ": {"ATA", "Antarctica"},
	"AR": {"ARG", "Argentina"},
	"AS": {"ASM", "American Samoa"},
	"AT": {"AUT", "Austria"},
	"AU": {"AUS", "Australia"},
	"AW": {"ABW", "Aruba"},
	"AX": {"ALA", "Åland Islands"},
	"AZ": {"AZE", "Azerbaijan"},
	"BA": {"BIH", "Bosnia and Herzegovina"},
	"BB": {"BRB", "Barbados"},
	"BD": {"BGD", "Bangladesh"},
	"BE": {"BEL", "Belgium"},
	"BF": {"BFA", "Burkina Faso"},
	"BG": {"BGR", "Bulgaria"},
	"BH": {"BHR", "Bahrain"},
	"BI": {"BDI", "Burundi"},
	"BJ": {"BEN", "Benin"},
	"BL": {"BLM", "Saint Barthélemy"},
	"BM": {"BMU", "Bermuda"},
	"BN": {"BRN", "Brunei Darussalam"},
	"BO": {"BOL", "Bolivia"},
	"BQ": {"BES", "Bonaire, Sint Eustatius and Saba"},
	"BR": {"BRA", "Brazil"},
	"BS": {"BHS", "Bahamas"},
	"BT": {"BTN", "Bhutan"},
	"BV": {"BVT", "Bouvet Island"},
	"BW": {"BWA", "Botswana"},
	"BY": {"BLR", "Belarus"},
	"BZ": {"BLZ", "Belize"},
	"CA": {"CAN", "Canada"},
	"CC": {"CCK", "Cocos (Keeling) Islands"},
	"CD": {"COD", "Congo, Democratic Republic of the"},
	"CF": {"CAF", "Central African Republic"},
	"CG": {"COG", "Congo"},
	"CH": {"CHE", "Switzerland"},
	"CI": {"CIV", "Côte d'Ivoire"},
	"CK": {"COK", "Cook Islands"},
	"CL": {"CHL", "Chile"},
	"CM": {"CMR", "Cameroon"},
	"CN": {"CHN", "China"},
	"CO": {"COL", "Colombia"},
	"CR": {"CRI", "Costa Rica"},
	"CU": {"CUB", "Cuba"},
	"CV": {"CPV", "Cabo Verde"},
	"CW": {"CUW", "Curaçao"},
	"CX": {"CXR", "Christmas Island"},
	"CY": {"CYP", "Cyprus"},
	"CZ": {"CZE", "Czechia"},
	"DE": {"DEU", "Germany"},
	"DJ": {"DJI", "Djibouti"},
	"DK": {"DNK", "Denmark"},
	"DM": {"DMA", "Dominica"},
	"DO": {"DOM", "Dominican Republic"},
	"DZ": {"DZA", "Algeria"},
	"EC": {"ECU", "Ecuador"},
	"EE": {"EST", "Estonia"},
	"EG": {"EGY", "Egypt"},
	"EH": {"ESH", "Western Sahara"},
	"ER": {"ERI", "Eritrea"},
	"ES": {"ESP", "Spain"},
	"ET": {"ETH", "Ethiopia"},
	"FI": {"FIN", "Finland"},
	"FJ": {"FJI", "Fiji"},
	"FK": {"FLK", "Falkland Islands (Malvinas)"},
	"FM": {"FSM", "Micronesia, Federated States of"},
	"FO": {"FRO", "Faroe Islands"},
	"FR": {"FRA", "France"},
	"GA": {"GAB", "Gabon"},
	"GB": {"GBR", "United Kingdom"},
	"GD": {"GRD", "Grenada"},
	"GE": {"GEO", "Georgia"},
	"GF": {"GUF", "French Guiana"},
	"GG": {"GGY", "Guernsey"},
	"GH": {"GHA", "Ghana"},
	"GI": {"GIB", "Gibraltar"},
	"GL": {"GRL", "Greenland"},
	"GM": {"GMB", "Gambia"},
	"GN": {"GIN", "Guinea"},
	"GP": {"GLP", "Guadeloupe"},
	"GQ": {"GNQ", "Equatorial Guinea"},
	"GR": {"GRC", "Greece"},
	"GS": {"SGS", "South Georgia and the South Sandwich Islands"},
	"GT": {"GTM", "Guatemala"},
	"GU": {"GUM", "Guam"},
	"GW": {"GNB", "Guinea-Bissau"},
	"GY": {"GUY", "Guyana"},
	"HK": {"HKG", "Hong Kong"},
	"HM": {"HMD", "Heard Island and McDonald Islands"},
	"HN": {"HND", "Honduras"},
	"HR": {"HRV", "Croatia"},
	"HT": {"HTI", "Haiti"},
	"HU": {"HUN", "Hungary"},
	"ID": {"IDN", "Indonesia"},
	"IE": {"IRL", "Ireland"},
	"IL": {"ISR", "Israel"},
	"IM": {"IMN", "Isle of Man"},
	"IN": {"IND", "India"},
	"IO": {"IOT", "British Indian Ocean Territory"},
	"IQ": {"IRQ", "Iraq"},
	"IR": {"IRN", "Iran, Islamic Republic of"},
	"IS": {"ISL", "Iceland"},
	"IT": {"ITA", "Italy"},
	"JE": {"JEY", "Jersey"},
	"JM": {"JAM", "Jamaica"},
	"JO": {"JOR", "Jordan"},
	"JP": {"JPN", "Japan"},
	"KE": {"KEN", "Kenya"},
	"KG": {"KGZ", "Kyrgyzstan"},
	"KH": {"KHM", "Cambodia"},
	"KI": {"KIR", "Kiribati"},
	"KM": {"COM", "Comoros"},
	"KN": {"KNA", "Saint Kitts and Nevis"},
	"KP": {"PRK", "Korea, Democratic People's Republic of"},
	"KR": {"KOR", "Korea, Republic of"},
	"KW": {"KWT", "Kuwait"},
	"KY": {"CYM", "Cayman Islands"},
	"KZ": {"KAZ", "Kazakhstan"},
	"LA": {"LAO", "Lao People's Democratic Republic"},
	"LB": {"LBN", "Lebanon"},
	"LC": {"LCA", "Saint Lucia"},
	"LI": {"LIE", "Liechtenstein"},
	"LK": {"LKA", "Sri Lanka"},
	"LR": {"LBR", "Liberia"},
	"LS": {"LSO", "Lesotho"},
	"LT": {"LTU", "Lithuania"},
	"LU": {"LUX", "Luxembourg"},
	"LV": {"LVA", "Latvia"},
	"LY": {"LBY", "Libya"},
	"MA": {"MAR", "Morocco"},
	"MC": {"MCO", "Monaco"},
	"MD": {"MDA", "Moldova, Republic of"},
	"ME": {"MNE", "Montenegro"},
	"MF": {"MAF", "Saint Martin (French part)"},
	"MG": {"MDG", "Madagascar"},
	"MH": {"MHL", "Marshall Islands"},
	"MK": {"MKD", "North Macedonia"},
	"ML": {"MLI", "Mali"},
	"MM": {"MMR", "Myanmar"},
	"MN": {"MNG", "Mongolia"},
	"MO": {"MAC", "Macao"},
	"MP": {"MNP", "Northern Mariana Islands"},
	"MQ": {"MTQ", "Martinique"},
	"MR": {"MRT", "Mauritania"},
	"MS": {"MSR", "Montserrat"},
	"MT": {"MLT", "Malta"},
	"MU": {"MUS", "Mauritius"},
	"MV": {"MDV", "Maldives"},
	"MW": {"MWI", "Malawi"},
	"MX": {"MEX", "Mexico"},
	"MY": {"MYS", "Malaysia"},
	"MZ": {"MOZ", "Mozambique"},
	"NA": {"NAM", "Namibia"},
	"NC": {"NCL", "New Caledonia"},
	"NE": {"NER", "Niger"},
	"NF": {"NFK", "Norfolk Island"},
	"NG": {"NGA", "Nigeria"},
	"NI": {"NIC", "Nicaragua"},
	"NL": {"NLD", "Netherlands"},
	"NO": {"NOR", "Norway"},
	"NP": {"NPL", "Nepal"},
	"NR": {"NRU", "Nauru"},
	"NU": {"NIU", "Niue"},
	"NZ": {"NZL", "New Zealand"},
	"OM": {"OMN", "Oman"},
	"PA": {"PAN", "Panama"},
	"PE": {"PER", "Peru"},
	"PF": {"PYF", "French Polynesia"},
	"PG": {"PNG", "Papua New Guinea"},
	"PH": {"PHL", "Philippines"},
	"PK": {"PAK", "Pakistan"},
	"PL": {"POL", "Poland"},
	"PM": {"SPM", "Saint Pierre and Miquelon"},
	"PN": {"PCN", "Pitcairn"},
	"PR": {"PRI", "Puerto Rico"},
	"PS": {"PSE", "Palestine, State of"},
	"PT": {"PRT", "Portugal"},
	"PW": {"PLW", "Palau"},
	"PY": {"PRY", "Paraguay"},
	"QA": {"QAT", "Qatar"},
	"RE": {"REU", "Réunion"},
	"RO": {"ROU", "Romania"},
	"RS": {"SRB", "Serbia"},
	"RU": {"RUS", "Russian Federation"},
	"RW": {"RWA", "Rwanda"},
	"SA": {"SAU", "Saudi Arabia"},
	"SB": {"SLB", "Solomon Islands"},
	"SC": {"SYC", "Seychelles"},
	"SD": {"SDN", "Sudan"},
	"SE": {"SWE", "Sweden"},
	"SG": {"SGP", "Singapore"},
	"SH": {"SHN", "Saint Helena, Ascension and Tristan da Cunha"},
	"SI": {"SVN", "Slovenia"},
	"SJ": {"SJM", "Svalbard and Jan Mayen"},
	"SK": {"SVK", "Slovakia"},
	"SL": {"SLE", "Sierra Leone"},
	"SM": {"SMR", "San Marino"},
	"SN": {"SEN", "Senegal"},
	"SO": {"SOM", "Somalia"},
	"SR": {"SUR", "Suriname"},
	"SS": {"SSD", "South Sudan"},
	"ST": {"STP", "Sao Tome and Principe"},
	"SV": {"SLV", "El Salvador"},
	"SX": {"SXM", "Sint Maarten (Dutch part)"},
	"SY": {"SYR", "Syrian Arab Republic"},
	"SZ": {"SWZ", "Eswatini"},
	"TC": {"TCA", "Turks and Caicos Islands"},
	"TD": {"TCD", "Chad"},
	"TF": {"ATF", "French Southern Territories"},
	"TG": {"TGO", "Togo"},
	"TH": {"THA", "Thailand"},
	"TJ": {"TJK", "Tajikistan"},
	"TK": {"TKL", "Tokelau"},
	"TL": {"TLS", "Timor-Leste"},
	"TM": {"TKM", "Turkmenistan"},
	"TN": {"TUN", "Tunisia"},
	"TO": {"TON", "Tonga"},
	"TR": {"TUR", "Turkey"},
	"TT": {"TTO", "Trinidad and Tobago"},
	"TV": {"TUV", "Tuvalu"},
	"TW": {"TWN", "Taiwan, Province of China"},
	"TZ": {"TZA", "Tanzania, United Republic of"},
	"UA": {"UKR", "Ukraine"},
	"UG": {"UGA", "Uganda"},
	"UM": {"UMI", "United States Minor Outlying Islands"},
	"US": {"USA", "United States"},
	"UY": {"URY", "Uruguay"},
	"UZ": {"UZB", "Uzbekistan"},
	"VA": {"VAT", "Holy See (Vatican City State)"},
	"VC": {"VCT", "Saint Vincent and the Grenadines"},
	"VE": {"VEN", "Venezuela, Bolivarian Republic of"},
	"VG": {"VGB", "Virgin Islands, British"},
	"VI": {"VIR", "Virgin Islands, U.S."},
	"VN": {"VNM", "Viet Nam"},
	"VU": {"VUT", "Vanuatu"},
	"WF": {"WLF", "Wallis and Futuna"},
	"WS": {"WSM", "Samoa"},
	"YE": {"YEM", "Yemen"},
	"YT": {"MYT", "Mayotte"},
	"ZA": {"ZAF", "South Africa"},
	"ZM": {"ZMB", "Zambia"},
	"ZW": {"ZWE", "Zimbabwe"},
}

// isoAlpha3ToAlpha2 is derived from isoAlpha2ToCountry by inversion.
var isoAlpha3ToAlpha2 = func() map[string]string {
	m := make(map[string]string, len(isoAlpha2ToCountry))
	for a2, c := range isoAlpha2ToCountry {
		m[c.Alpha3] = a2
	}
	return m
}()

// isoNameVariants maps common spellings / aliases → ISO alpha-2. Covers "USA",
// "UK", "U.S.", "Great Britain", full official names, etc. Keys are lowercased
// for case-insensitive lookup.
var isoNameVariants = map[string]string{
	"united states":                         "US",
	"united states of america":              "US",
	"usa":                                   "US",
	"u.s.":                                  "US",
	"u.s.a.":                                "US",
	"america":                               "US",
	"united kingdom":                        "GB",
	"uk":                                    "GB",
	"u.k.":                                  "GB",
	"great britain":                         "GB",
	"britain":                               "GB",
	"england":                               "GB",
	"russia":                                "RU",
	"russian federation":                    "RU",
	"south korea":                           "KR",
	"korea":                                 "KR",
	"republic of korea":                     "KR",
	"north korea":                           "KP",
	"democratic people's republic of korea": "KP",
	"iran":                                  "IR",
	"iran, islamic republic of":             "IR",
	"syria":                                 "SY",
	"syrian arab republic":                  "SY",
	"vietnam":                               "VN",
	"viet nam":                              "VN",
	"czech republic":                        "CZ",
	"czechia":                               "CZ",
	"north macedonia":                       "MK",
	"macedonia":                             "MK",
	"burma":                                 "MM",
	"myanmar":                               "MM",
	"taiwan":                                "TW",
	"taiwan, province of china":             "TW",
	"holland":                               "NL",
	"netherlands":                           "NL",
	"the netherlands":                       "NL",
	"eswatini":                              "SZ",
	"swaziland":                             "SZ",
	"cabo verde":                            "CV",
	"cape verde":                            "CV",
	"reunion":                               "RE",
	"réunion":                               "RE",
	"south sudan":                           "SS",
	"ivory coast":                           "CI",
	"côte d'ivoire":                         "CI",
	"turkey":                                "TR",
	"türkiye":                               "TR",
	"saudi arabia":                          "SA",
	"united arab emirates":                  "AE",
	"uae":                                   "AE",
	"new zealand":                           "NZ",
	"south africa":                          "ZA",
	"sri lanka":                             "LK",
	"sierra leone":                          "SL",
	"el salvador":                           "SV",
	"trinidad and tobago":                   "TT",
	"trinidad & tobago":                     "TT",
	"antigua and barbuda":                   "AG",
	"bosnia and herzegovina":                "BA",
	"saint vincent and the grenadines":      "VC",
	"saint kitts and nevis":                 "KN",
	"saint lucia":                           "LC",
}

// countryNameToAlpha2Lookup resolves a country name (canonical or common
// variant) to its ISO 3166-1 alpha-2 code. Returns "" if unknown.
func countryNameToAlpha2Lookup(name string) string {
	if name == "" {
		return ""
	}
	low := strings.ToLower(strings.TrimSpace(name))
	if low == "" {
		return ""
	}
	// Try the literal spelling first (handles "U.S.A.", "U.K.", etc.), then a
	// trailing-dot-stripped form as a fallback (handles "United States.").
	if a2, ok := isoNameVariants[low]; ok {
		return a2
	}
	stripped := strings.TrimSuffix(low, ".")
	if stripped != low {
		if a2, ok := isoNameVariants[stripped]; ok {
			return a2
		}
	}
	// Walk the canonical table by name.
	for a2, c := range isoAlpha2ToCountry {
		if strings.ToLower(c.Name) == low {
			return a2
		}
	}
	return ""
}

// ---- country_alpha2_to_alpha3 ----

type CountryAlpha2ToAlpha3Skill struct{ *kyoci.BaseSkill }

func NewCountryAlpha2ToAlpha3Skill() *CountryAlpha2ToAlpha3Skill {
	return &CountryAlpha2ToAlpha3Skill{BaseSkill: kyoci.NewBaseSkill(
		"country_alpha2_to_alpha3",
		"Convert ISO 3166-1 alpha-2 country code to alpha-3. Usage: 'country alpha2 to alpha3: US'",
		[]string{"country alpha2 to alpha3", "country code to alpha3", "alpha2 to alpha3"},
	)}
}

func (s *CountryAlpha2ToAlpha3Skill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "country alpha2 to alpha3") ||
		strings.Contains(low, "country code to alpha3") ||
		strings.Contains(low, "alpha2 to alpha3") ||
		strings.Contains(low, "alpha 2 to alpha 3")
}

func (s *CountryAlpha2ToAlpha3Skill) Execute(_ context.Context, q string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(extractPayload(q)))
	if code == "" {
		return "", fmt.Errorf("expected an alpha-2 country code")
	}
	c, ok := isoAlpha2ToCountry[code]
	if !ok {
		return "", fmt.Errorf("unknown alpha-2 country code %q", code)
	}
	return fmt.Sprintf("%s → %s (%s)", code, c.Alpha3, c.Name), nil
}

// ---- country_alpha3_to_alpha2 ----

type CountryAlpha3ToAlpha2Skill struct{ *kyoci.BaseSkill }

func NewCountryAlpha3ToAlpha2Skill() *CountryAlpha3ToAlpha2Skill {
	return &CountryAlpha3ToAlpha2Skill{BaseSkill: kyoci.NewBaseSkill(
		"country_alpha3_to_alpha2",
		"Convert ISO 3166-1 alpha-3 country code to alpha-2. Usage: 'country alpha3 to alpha2: USA'",
		[]string{"country alpha3 to alpha2", "alpha3 to alpha2"},
	)}
}

func (s *CountryAlpha3ToAlpha2Skill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "country alpha3 to alpha2") ||
		strings.Contains(low, "alpha3 to alpha2") ||
		strings.Contains(low, "alpha 3 to alpha 2")
}

func (s *CountryAlpha3ToAlpha2Skill) Execute(_ context.Context, q string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(extractPayload(q)))
	if code == "" {
		return "", fmt.Errorf("expected an alpha-3 country code")
	}
	a2, ok := isoAlpha3ToAlpha2[code]
	if !ok {
		return "", fmt.Errorf("unknown alpha-3 country code %q", code)
	}
	c := isoAlpha2ToCountry[a2]
	return fmt.Sprintf("%s → %s (%s)", code, a2, c.Name), nil
}

// ---- country_name_to_alpha2 ----

type CountryNameToAlpha2Skill struct{ *kyoci.BaseSkill }

func NewCountryNameToAlpha2Skill() *CountryNameToAlpha2Skill {
	return &CountryNameToAlpha2Skill{BaseSkill: kyoci.NewBaseSkill(
		"country_name_to_alpha2",
		"Convert a country name (with common variants) to ISO 3166-1 alpha-2. "+
			"Usage: 'country name to alpha2: United States'",
		[]string{"country name to alpha2", "country name to code", "name to alpha2"},
	)}
}

func (s *CountryNameToAlpha2Skill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "country name to alpha2") ||
		strings.Contains(low, "country name to code") ||
		strings.Contains(low, "name to alpha2") ||
		strings.Contains(low, "name to code")
}

func (s *CountryNameToAlpha2Skill) Execute(_ context.Context, q string) (string, error) {
	name := strings.TrimSpace(extractPayload(q))
	name = strings.Trim(name, "\"'`")
	if name == "" {
		return "", fmt.Errorf("expected a country name")
	}
	a2 := countryNameToAlpha2Lookup(name)
	if a2 == "" {
		return "", fmt.Errorf("unknown country name %q", name)
	}
	c := isoAlpha2ToCountry[a2]
	return fmt.Sprintf("%s → %s (%s)", name, a2, c.Name), nil
}

// ---- country_alpha2_to_name ----

type CountryAlpha2ToNameSkill struct{ *kyoci.BaseSkill }

func NewCountryAlpha2ToNameSkill() *CountryAlpha2ToNameSkill {
	return &CountryAlpha2ToNameSkill{BaseSkill: kyoci.NewBaseSkill(
		"country_alpha2_to_name",
		"Convert ISO 3166-1 alpha-2 country code to its full name. Usage: 'country alpha2 to name: US'",
		[]string{"country alpha2 to name", "alpha2 to name", "country code to name"},
	)}
}

func (s *CountryAlpha2ToNameSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "country alpha2 to name") ||
		strings.Contains(low, "country code to name") ||
		strings.Contains(low, "alpha2 to name")
}

func (s *CountryAlpha2ToNameSkill) Execute(_ context.Context, q string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(extractPayload(q)))
	if code == "" {
		return "", fmt.Errorf("expected an alpha-2 country code")
	}
	c, ok := isoAlpha2ToCountry[code]
	if !ok {
		return "", fmt.Errorf("unknown alpha-2 country code %q", code)
	}
	return fmt.Sprintf("%s → %s (%s)", code, c.Name, c.Alpha3), nil
}

// =====================================================================================
// ISO 4217 currency tables.
// =====================================================================================

// isoCountryCurrency maps ISO 3166-1 alpha-2 → ISO 4217 currency code. Covers
// all UN member states plus major territories.
var isoCountryCurrency = map[string]string{
	"AD": "EUR", "AE": "AED", "AF": "AFN", "AG": "XCD", "AI": "XCD",
	"AL": "ALL", "AM": "AMD", "AO": "AOA", "AR": "ARS", "AT": "EUR",
	"AU": "AUD", "AW": "AWG", "AX": "EUR", "AZ": "AZN", "BA": "BAM",
	"BB": "BBD", "BD": "BDT", "BE": "EUR", "BF": "XOF", "BG": "BGN",
	"BH": "BHD", "BI": "BIF", "BJ": "XOF", "BL": "EUR", "BM": "BMD",
	"BN": "BND", "BO": "BOB", "BQ": "USD", "BR": "BRL", "BS": "BSD",
	"BT": "BTN", "BW": "BWP", "BY": "BYN", "BZ": "BZD", "CA": "CAD",
	"CD": "CDF", "CF": "XAF", "CG": "XAF", "CH": "CHF", "CI": "XOF",
	"CK": "NZD", "CL": "CLP", "CM": "XAF", "CN": "CNY", "CO": "COP",
	"CR": "CRC", "CU": "CUP", "CV": "CVE", "CW": "ANG", "CY": "EUR",
	"CZ": "CZK", "DE": "EUR", "DJ": "DJF", "DK": "DKK", "DM": "XCD",
	"DO": "DOP", "DZ": "DZD", "EC": "USD", "EE": "EUR", "EG": "EGP",
	"ER": "ERN", "ES": "EUR", "ET": "ETB", "FI": "EUR", "FJ": "FJD",
	"FK": "FKP", "FO": "DKK", "FR": "EUR", "GA": "XAF", "GB": "GBP",
	"GD": "XCD", "GE": "GEL", "GF": "EUR", "GG": "GBP", "GH": "GHS",
	"GI": "GIP", "GL": "DKK", "GM": "GMD", "GN": "GNF", "GP": "EUR",
	"GQ": "XAF", "GR": "EUR", "GT": "GTQ", "GU": "USD", "GW": "XOF",
	"GY": "GYD", "HK": "HKD", "HN": "HNL", "HR": "EUR", "HT": "HTG",
	"HU": "HUF", "ID": "IDR", "IE": "EUR", "IL": "ILS", "IM": "GBP",
	"IN": "INR", "IQ": "IQD", "IR": "IRR", "IS": "ISK", "IT": "EUR",
	"JE": "GBP", "JM": "JMD", "JO": "JOD", "JP": "JPY", "KE": "KES",
	"KG": "KGS", "KH": "KHR", "KI": "AUD", "KM": "KMF", "KN": "XCD",
	"KP": "KPW", "KR": "KRW", "KW": "KWD", "KY": "KYD", "KZ": "KZT",
	"LA": "LAK", "LB": "LBP", "LC": "XCD", "LI": "CHF", "LK": "LKR",
	"LR": "LRD", "LS": "LSL", "LT": "EUR", "LU": "EUR", "LV": "EUR",
	"LY": "LYD", "MA": "MAD", "MC": "EUR", "MD": "MDL", "ME": "EUR",
	"MF": "EUR", "MG": "MGA", "MH": "USD", "MK": "MKD", "ML": "XOF",
	"MM": "MMK", "MN": "MNT", "MO": "MOP", "MP": "USD", "MQ": "EUR",
	"MR": "MRU", "MS": "XCD", "MT": "EUR", "MU": "MUR", "MV": "MVR",
	"MW": "MWK", "MX": "MXN", "MY": "MYR", "MZ": "MZN", "NA": "NAD",
	"NC": "XPF", "NE": "XOF", "NG": "NGN", "NI": "NIO", "NL": "EUR",
	"NO": "NOK", "NP": "NPR", "NR": "AUD", "NZ": "NZD", "OM": "OMR",
	"PA": "USD", "PE": "PEN", "PF": "XPF", "PG": "PGK", "PH": "PHP",
	"PK": "PKR", "PL": "PLN", "PM": "EUR", "PR": "USD", "PS": "ILS",
	"PT": "EUR", "PW": "USD", "PY": "PYG", "QA": "QAR", "RE": "EUR",
	"RO": "RON", "RS": "RSD", "RU": "RUB", "RW": "RWF", "SA": "SAR",
	"SB": "SBD", "SC": "SCR", "SD": "SDG", "SE": "SEK", "SG": "SGD",
	"SH": "SHP", "SI": "EUR", "SJ": "NOK", "SK": "EUR", "SL": "SLL",
	"SM": "EUR", "SN": "XOF", "SO": "SOS", "SR": "SRD", "SS": "SSP",
	"ST": "STN", "SV": "USD", "SX": "ANG", "SY": "SYP", "SZ": "SZL",
	"TC": "USD", "TD": "XAF", "TG": "XOF", "TH": "THB", "TJ": "TJS",
	"TL": "USD", "TM": "TMT", "TN": "TND", "TO": "TOP", "TR": "TRY",
	"TT": "TTD", "TV": "AUD", "TW": "TWD", "TZ": "TZS", "UA": "UAH",
	"UG": "UGX", "US": "USD", "UY": "UYU", "UZ": "UZS", "VA": "EUR",
	"VC": "XCD", "VE": "VES", "VG": "USD", "VI": "USD", "VN": "VND",
	"VU": "VUV", "WF": "XPF", "WS": "WST", "YE": "YER", "YT": "EUR",
	"ZA": "ZAR", "ZM": "ZMW", "ZW": "ZWG",
}

// currencySymbols maps ISO 4217 currency code → common symbol. ~60 entries —
// the major traded currencies plus a healthy selection of regional ones.
var currencySymbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CNY": "¥",
	"RUB": "₽", "INR": "₹", "KRW": "₩", "BRL": "R$", "CHF": "Fr",
	"SEK": "kr", "NOK": "kr", "DKK": "kr", "PLN": "zł", "CZK": "Kč",
	"HUF": "Ft", "TRY": "₺", "ILS": "₪", "THB": "฿", "PHP": "₱",
	"VND": "₫", "IDR": "Rp", "MYR": "RM", "SGD": "S$", "HKD": "HK$",
	"TWD": "NT$", "AUD": "A$", "NZD": "NZ$", "CAD": "C$", "MXN": "$",
	"ARS": "$", "CLP": "$", "COP": "$", "PEN": "S/", "UYU": "$U",
	"ZAR": "R", "NGN": "₦", "EGP": "E£", "MAD": "DH", "AED": "د.إ",
	"SAR": "﷼", "QAR": "﷼", "KWD": "د.ك", "BHD": ".د.ب", "OMR": "﷼",
	"JOD": "د.ا", "LBP": "ل.ل", "PKR": "₨", "BDT": "৳", "LKR": "Rs",
	"NPR": "₨", "UAH": "₴", "RON": "lei", "BGN": "лв", "HRK": "kn",
	"RSD": "din", "ISK": "kr", "VES": "Bs", "DZD": "دج", "TND": "د.ت",
	"KZT": "₸", "VUV": "Vt", "WST": "T", "XPF": "₣", "XOF": "CFA",
	"XAF": "FCFA", "GHS": "₵",
}

// ---- currency_code_lookup ----

type CurrencyCodeLookupSkill struct{ *kyoci.BaseSkill }

func NewCurrencyCodeLookupSkill() *CurrencyCodeLookupSkill {
	return &CurrencyCodeLookupSkill{BaseSkill: kyoci.NewBaseSkill(
		"currency_code_lookup",
		"Look up ISO 4217 currency code for a country (alpha-2). Usage: 'currency code lookup: US'",
		[]string{"currency code lookup", "currency for country", "currency of country"},
	)}
}

func (s *CurrencyCodeLookupSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "currency code lookup") ||
		strings.Contains(low, "currency for country") ||
		strings.Contains(low, "currency of country") ||
		strings.Contains(low, "currency code for")
}

func (s *CurrencyCodeLookupSkill) Execute(_ context.Context, q string) (string, error) {
	payload := strings.TrimSpace(extractPayload(q))
	// Accept either an alpha-2 country code or a country name.
	if payload == "" {
		return "", fmt.Errorf("expected a country code or country name")
	}
	upper := strings.ToUpper(payload)
	if code, ok := isoCountryCurrency[upper]; ok {
		c := isoAlpha2ToCountry[upper]
		name := upper
		if c.Name != "" {
			name = c.Name
		}
		return fmt.Sprintf("%s → %s (%s)", name, code, currencyNameFor(code)), nil
	}
	// Try as a country name.
	a2 := countryNameToAlpha2Lookup(payload)
	if a2 == "" {
		return "", fmt.Errorf("unknown country %q", payload)
	}
	code := isoCountryCurrency[a2]
	c := isoAlpha2ToCountry[a2]
	return fmt.Sprintf("%s → %s (%s)", c.Name, code, currencyNameFor(code)), nil
}

// currencyNameFor returns a short human name for common ISO 4217 codes. Falls
// back to the code itself if unknown — used only to enrich output.
func currencyNameFor(code string) string {
	names := map[string]string{
		"USD": "US Dollar", "EUR": "Euro", "GBP": "Pound Sterling",
		"JPY": "Yen", "CNY": "Yuan Renminbi", "INR": "Indian Rupee",
		"AUD": "Australian Dollar", "CAD": "Canadian Dollar",
		"CHF": "Swiss Franc", "ZAR": "Rand", "BRL": "Brazilian Real",
		"RUB": "Ruble", "KRW": "Won", "MXN": "Mexican Peso",
		"SGD": "Singapore Dollar", "HKD": "Hong Kong Dollar",
		"NZD": "New Zealand Dollar", "SEK": "Swedish Krona",
		"NOK": "Norwegian Krone", "DKK": "Danish Krone", "PLN": "Zloty",
		"THB": "Baht", "IDR": "Rupiah", "MYR": "Malaysian Ringgit",
		"PHP": "Philippine Peso", "VND": "Dong", "CZK": "Czech Koruna",
		"HUF": "Forint", "ILS": "Shekel", "TRY": "Turkish Lira",
		"AED": "UAE Dirham", "SAR": "Saudi Riyal", "EGP": "Egyptian Pound",
		"NGN": "Naira", "PKR": "Pakistan Rupee", "BDT": "Taka",
		"UAH": "Hryvnia", "RON": "Romanian Leu", "CLP": "Chilean Peso",
		"COP": "Colombian Peso", "PEN": "Sol", "ARS": "Argentine Peso",
	}
	if n, ok := names[code]; ok {
		return n
	}
	return code
}

// ---- currency_symbol ----

type CurrencySymbolSkill struct{ *kyoci.BaseSkill }

func NewCurrencySymbolSkill() *CurrencySymbolSkill {
	return &CurrencySymbolSkill{BaseSkill: kyoci.NewBaseSkill(
		"currency_symbol",
		"Get the common symbol for an ISO 4217 currency code. Usage: 'currency symbol: USD'",
		[]string{"currency symbol", "symbol for currency"},
	)}
}

func (s *CurrencySymbolSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "currency symbol") ||
		strings.Contains(low, "symbol for currency") ||
		strings.Contains(low, "symbol of currency")
}

func (s *CurrencySymbolSkill) Execute(_ context.Context, q string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(extractPayload(q)))
	if code == "" {
		return "", fmt.Errorf("expected an ISO 4217 currency code")
	}
	sym, ok := currencySymbols[code]
	if !ok {
		return "", fmt.Errorf("no symbol recorded for currency %q", code)
	}
	return fmt.Sprintf("%s → %s", code, sym), nil
}
