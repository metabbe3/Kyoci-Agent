package builtin

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	kyoci "github.com/metabbe3/Kyoci-Agent/pkg"
)

// =====================================================================================
// Lookup-table skills — zero-AI reference data baked into the binary.
//
// Each skill returns a STATIC multi-line reference table (ISO codes, HTTP
// statuses, MIME types, ASCII glyphs, UUID namespaces, Unix signals, magic
// bytes, emoji shortcodes). No network, no external lookups — the data lives
// in this file as a Go string literal.
// =====================================================================================

// ---- ISO 3166-1 alpha-2 country codes ----

type ISOCountryAlpha2ListSkill struct{ *kyoci.BaseSkill }

func NewISOCountryAlpha2ListSkill() *ISOCountryAlpha2ListSkill {
	return &ISOCountryAlpha2ListSkill{BaseSkill: kyoci.NewBaseSkill(
		"iso_country_alpha2_list", "Return ISO 3166-1 alpha-2 country codes (one per line)",
		[]string{"iso country alpha2", "list countries alpha2", "alpha2 country codes"},
	)}
}
func (s *ISOCountryAlpha2ListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "iso country alpha2") ||
		strings.Contains(low, "list countries alpha2") ||
		strings.Contains(low, "alpha2 country codes") ||
		strings.Contains(low, "country alpha2")
}
func (s *ISOCountryAlpha2ListSkill) Execute(_ context.Context, _ string) (string, error) {
	return isoCountryAlpha2Table, nil
}

const isoCountryAlpha2Table = `AD Andorra
AE United Arab Emirates
AF Afghanistan
AG Antigua and Barbuda
AL Albania
AM Armenia
AR Argentina
AT Austria
AU Australia
AZ Azerbaijan
BA Bosnia and Herzegovina
BB Barbados
BD Bangladesh
BE Belgium
BG Bulgaria
BH Bahrain
BN Brunei Darussalam
BO Bolivia
BR Brazil
BS Bahamas
BW Botswana
BY Belarus
BZ Belize
CA Canada
CH Switzerland
CL Chile
CN China
CO Colombia
CR Costa Rica
CU Cuba
CY Cyprus
CZ Czechia
DE Germany
DK Denmark
DO Dominican Republic
DZ Algeria
EC Ecuador
EE Estonia
EG Egypt
ES Spain
ET Ethiopia
FI Finland
FJ Fiji
FR France
GB United Kingdom
GE Georgia
GH Ghana
GR Greece
GT Guatemala
HK Hong Kong
HR Croatia
HU Hungary
ID Indonesia
IE Ireland
IL Israel
IN India
IQ Iraq
IR Iran
IS Iceland
IT Italy
JM Jamaica
JO Jordan
JP Japan
KE Kenya
KH Cambodia
KR South Korea
KW Kuwait
KZ Kazakhstan
LA Laos
LB Lebanon
LI Liechtenstein
LK Sri Lanka
LT Lithuania
LU Luxembourg
LV Latvia
LY Libya
MA Morocco
MC Monaco
MD Moldova
ME Montenegro
MG Madagascar
MK North Macedonia
MM Myanmar
MN Mongolia
MO Macao
MT Malta
MV Maldives
MX Mexico
MY Malaysia
NA Namibia
NG Nigeria
NI Nicaragua
NL Netherlands
NO Norway
NP Nepal
NZ New Zealand
OM Oman
PA Panama
PE Peru
PG Papua New Guinea
PH Philippines
PK Pakistan
PL Poland
PR Puerto Rico
PT Portugal
PY Paraguay
QA Qatar
RO Romania
RS Serbia
RU Russia
RW Rwanda
SA Saudi Arabia
SE Sweden
SG Singapore
SI Slovenia
SK Slovakia
SN Senegal
SO Somalia
SR Suriname
SD Sudan
SY Syria
TH Thailand
TJ Tajikistan
TM Turkmenistan
TN Tunisia
TR Turkey
TT Trinidad and Tobago
TW Taiwan
TZ Tanzania
UA Ukraine
UG Uganda
US United States
UY Uruguay
UZ Uzbekistan
VE Venezuela
VN Vietnam
YE Yemen
ZA South Africa
ZM Zambia
ZW Zimbabwe`

// ---- ISO 3166-1 alpha-3 country codes ----

type ISOCountryAlpha3ListSkill struct{ *kyoci.BaseSkill }

func NewISOCountryAlpha3ListSkill() *ISOCountryAlpha3ListSkill {
	return &ISOCountryAlpha3ListSkill{BaseSkill: kyoci.NewBaseSkill(
		"iso_country_alpha3_list", "Return ISO 3166-1 alpha-3 country codes (one per line)",
		[]string{"iso country alpha3", "list countries alpha3", "alpha3 country codes"},
	)}
}
func (s *ISOCountryAlpha3ListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "iso country alpha3") ||
		strings.Contains(low, "list countries alpha3") ||
		strings.Contains(low, "alpha3 country codes") ||
		strings.Contains(low, "country alpha3")
}
func (s *ISOCountryAlpha3ListSkill) Execute(_ context.Context, _ string) (string, error) {
	return isoCountryAlpha3Table, nil
}

const isoCountryAlpha3Table = `AND Andorra
ARE United Arab Emirates
AFG Afghanistan
ATG Antigua and Barbuda
ALB Albania
ARM Armenia
ARG Argentina
AUT Austria
AUS Australia
AZE Azerbaijan
BIH Bosnia and Herzegovina
BRB Barbados
BGD Bangladesh
BEL Belgium
BGR Bulgaria
BHR Bahrain
BRN Brunei Darussalam
BOL Bolivia
BRA Brazil
BHS Bahamas
BWA Botswana
BLR Belarus
BLZ Belize
CAN Canada
CHE Switzerland
CHL Chile
CHN China
COL Colombia
CRI Costa Rica
CUB Cuba
CYP Cyprus
CZE Czechia
DEU Germany
DNK Denmark
DOM Dominican Republic
DZA Algeria
ECU Ecuador
EST Estonia
EGY Egypt
ESP Spain
ETH Ethiopia
FIN Finland
FJI Fiji
FRA France
GBR United Kingdom
GEO Georgia
GHA Ghana
GRC Greece
GTM Guatemala
HKG Hong Kong
HRV Croatia
HUN Hungary
IDN Indonesia
IRL Ireland
ISR Israel
IND India
IRQ Iraq
IRN Iran
ISL Iceland
ITA Italy
JAM Jamaica
JOR Jordan
JPN Japan
KEN Kenya
KHM Cambodia
KOR South Korea
KWT Kuwait
KAZ Kazakhstan
LAO Laos
LBN Lebanon
LIE Liechtenstein
LKA Sri Lanka
LTU Lithuania
LUX Luxembourg
LVA Latvia
LBY Libya
MAR Morocco
MCO Monaco
MDA Moldova
MNE Montenegro
MDG Madagascar
MKD North Macedonia
MMR Myanmar
MNG Mongolia
MAC Macao
MLT Malta
MDV Maldives
MEX Mexico
MYS Malaysia
NAM Namibia
NGA Nigeria
NIC Nicaragua
NLD Netherlands
NOR Norway
NPL Nepal
NZL New Zealand
OMN Oman
PAN Panama
PER Peru
PNG Papua New Guinea
PHL Philippines
PAK Pakistan
POL Poland
PRI Puerto Rico
PRT Portugal
PRY Paraguay
QAT Qatar
ROU Romania
SRB Serbia
RUS Russia
RWA Rwanda
SAU Saudi Arabia
SWE Sweden
SGP Singapore
SVN Slovenia
SVK Slovakia
SEN Senegal
SOM Somalia
SUR Suriname
SDN Sudan
SYR Syria
THA Thailand
TJK Tajikistan
TKM Turkmenistan
TUN Tunisia
TUR Turkey
TTO Trinidad and Tobago
TWN Taiwan
TZA Tanzania
UKR Ukraine
UGA Uganda
USA United States
URY Uruguay
UZB Uzbekistan
VEN Venezuela
VNM Vietnam
YEM Yemen
ZAF South Africa
ZMB Zambia
ZWE Zimbabwe`

// ---- ISO 4217 currency codes ----

type ISOCurrencyListSkill struct{ *kyoci.BaseSkill }

func NewISOCurrencyListSkill() *ISOCurrencyListSkill {
	return &ISOCurrencyListSkill{BaseSkill: kyoci.NewBaseSkill(
		"iso_currency_list", "Return ISO 4217 currency codes (code, name, minor unit)",
		[]string{"iso currency", "currency list", "list currencies"},
	)}
}
func (s *ISOCurrencyListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "iso currency") ||
		strings.Contains(low, "currency list") ||
		strings.Contains(low, "list currencies") ||
		strings.Contains(low, "currency codes")
}
func (s *ISOCurrencyListSkill) Execute(_ context.Context, _ string) (string, error) {
	return isoCurrencyTable, nil
}

const isoCurrencyTable = `Code  Name                         Minor
AED   UAE Dirham                   2
AFN   Afghan Afghani               2
ALL   Albanian Lek                 2
AMD   Armenian Dram                2
ARS   Argentine Peso               2
AUD   Australian Dollar            2
AZN   Azerbaijani Manat            2
BAM   Bosnia Mark                  2
BDT   Bangladeshi Taka             2
BGN   Bulgarian Lev                2
BHD   Bahraini Dinar               3
BND   Brunei Dollar                2
BOB   Bolivian Boliviano           2
BRL   Brazilian Real               2
BTC   Bitcoin (unofficial)         8
BWP   Botswana Pula                2
BYN   Belarusian Ruble             2
CAD   Canadian Dollar              2
CHF   Swiss Franc                  2
CLP   Chilean Peso                 0
CNY   Chinese Yuan                 2
COP   Colombian Peso               2
CRC   Costa Rican Colon            2
CZK   Czech Koruna                 2
DKK   Danish Krone                 2
DOP   Dominican Peso               2
DZD   Algerian Dinar               2
EGP   Egyptian Pound               2
ETB   Ethiopian Birr               2
EUR   Euro                         2
GBP   Pound Sterling               2
GEL   Georgian Lari                2
GHS   Ghanaian Cedi                2
GTQ   Guatemalan Quetzal           2
HKD   Hong Kong Dollar             2
HNL   Honduran Lempira             2
HRK   Croatian Kuna                2
HUF   Hungarian Forint             2
IDR   Indonesian Rupiah            2
ILS   Israeli Shekel               2
INR   Indian Rupee                 2
IQD   Iraqi Dinar                  3
IRR   Iranian Rial                 2
ISK   Icelandic Krona              0
JMD   Jamaican Dollar              2
JOD   Jordanian Dinar              3
JPY   Japanese Yen                 0
KES   Kenyan Shilling              2
KRW   South Korean Won             0
KWD   Kuwaiti Dinar                3
KZT   Kazakhstani Tenge            2
LBP   Lebanese Pound               2
LKR   Sri Lankan Rupee             2
LYD   Libyan Dinar                 3
MAD   Moroccan Dirham              2
MDL   Moldovan Leu                 2
MXN   Mexican Peso                 2
MYR   Malaysian Ringgit            2
MZN   Mozambican Metical           2
NGN   Nigerian Naira               2
NIO   Nicaraguan Cordoba           2
NOK   Norwegian Krone              2
NPR   Nepalese Rupee               2
NZD   New Zealand Dollar           2
OMR   Omani Rial                   3
PAB   Panamanian Balboa            2
PEN   Peruvian Sol                 2
PHP   Philippine Peso              2
PKR   Pakistani Rupee              2
PLN   Polish Zloty                 2
PYG   Paraguayan Guarani           0
QAR   Qatari Riyal                 2
RON   Romanian Leu                 2
RSD   Serbian Dinar                2
RUB   Russian Ruble                2
SAR   Saudi Riyal                  2
SEK   Swedish Krona                2
SGD   Singapore Dollar             2
SOS   Somali Shilling              2
SYP   Syrian Pound                 2
THB   Thai Baht                    2
TJS   Tajikistani Somoni           2
TND   Tunisian Dinar               3
TRY   Turkish Lira                 2
TTD   Trinidad Dollar              2
TWD   Taiwan Dollar                2
TZS   Tanzanian Shilling           2
UAH   Ukrainian Hryvnia            2
UGX   Ugandan Shilling             0
USD   US Dollar                    2
UYU   Uruguayan Peso               2
UZS   Uzbekistani Som              2
VES   Venezuelan Bolivar           2
VND   Vietnamese Dong              0
YER   Yemeni Rial                  2
ZAR   South African Rand           2
ZMW   Zambian Kwacha               2`

// ---- ISO 639-1 language codes ----

type ISOLanguageAlpha2ListSkill struct{ *kyoci.BaseSkill }

func NewISOLanguageAlpha2ListSkill() *ISOLanguageAlpha2ListSkill {
	return &ISOLanguageAlpha2ListSkill{BaseSkill: kyoci.NewBaseSkill(
		"iso_language_alpha2_list", "Return ISO 639-1 alpha-2 language codes",
		[]string{"iso language", "language alpha2", "language codes"},
	)}
}
func (s *ISOLanguageAlpha2ListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "iso language") ||
		strings.Contains(low, "language alpha2") ||
		strings.Contains(low, "language codes") ||
		strings.Contains(low, "list languages")
}
func (s *ISOLanguageAlpha2ListSkill) Execute(_ context.Context, _ string) (string, error) {
	return isoLanguageTable, nil
}

const isoLanguageTable = `Code  Name
aa    Afar
ab    Abkhazian
af    Afrikaans
ak    Akan
am    Amharic
ar    Arabic
as    Assamese
az    Azerbaijani
be    Belarusian
bg    Bulgarian
bm    Bambara
bn    Bengali
bo    Tibetan
br    Breton
bs    Bosnian
ca    Catalan
cs    Czech
cy    Welsh
da    Danish
de    German
el    Greek
en    English
eo    Esperanto
es    Spanish
et    Estonian
eu    Basque
fa    Persian
fi    Finnish
fj    Fijian
fo    Faroese
fr    French
fy    Western Frisian
ga    Irish
gd    Scottish Gaelic
gl    Galician
gn    Guarani
gu    Gujarati
gv    Manx
ha    Hausa
he    Hebrew
hi    Hindi
hr    Croatian
ht    Haitian Creole
hu    Hungarian
hy    Armenian
id    Indonesian
ig    Igbo
ii    Sichuan Yi
is    Icelandic
it    Italian
iu    Inuktitut
ja    Japanese
jv    Javanese
ka    Georgian
kk    Kazakh
kl    Kalaallisut
km    Khmer
kn    Kannada
ko    Korean
ks    Kashmiri
ku    Kurdish
kw    Cornish
ky    Kyrgyz
la    Latin
lb    Luxembourgish
lg    Ganda
ln    Lingala
lo    Lao
lt    Lithuanian
lu    Luba-Katanga
lv    Latvian
mg    Malagasy
mi    Maori
mk    Macedonian
ml    Malayalam
mn    Mongolian
mr    Marathi
ms    Malay
mt    Maltese
my    Burmese
nb    Norwegian Bokmal
ne    Nepali
nl    Dutch
nn    Norwegian Nynorsk
no    Norwegian
om    Oromo
or    Odia
os    Ossetian
pa    Panjabi
pl    Polish
ps    Pashto
pt    Portuguese
qu    Quechua
rm    Romansh
rn    Rundi
ro    Romanian
ru    Russian
rw    Kinyarwanda
sa    Sanskrit
sc    Sardinian
sd    Sindhi
se    Northern Sami
sg    Sango
si    Sinhala
sk    Slovak
sl    Slovenian
sn    Shona
so    Somali
sq    Albanian
sr    Serbian
ss    Swati
st    Southern Sotho
su    Sundanese
sv    Swedish
sw    Swahili
ta    Tamil
te    Telugu
tg    Tajik
th    Thai
ti    Tigrinya
tk    Turkmen
tl    Tagalog
tn    Tswana
to    Tongan
tr    Turkish
ts    Tsonga
tt    Tatar
ug    Uyghur
uk    Ukrainian
ur    Urdu
uz    Uzbek
ve    Venda
vi    Vietnamese
vo    Volapuk
wa    Walloon
wo    Wolof
xh    Xhosa
yi    Yiddish
yo    Yoruba
zh    Chinese
zu    Zulu`

// ---- HTTP status codes ----

type HTTPStatusAllSkill struct{ *kyoci.BaseSkill }

func NewHTTPStatusAllSkill() *HTTPStatusAllSkill {
	return &HTTPStatusAllSkill{BaseSkill: kyoci.NewBaseSkill(
		"http_status_all", "Return common HTTP status codes (1xx-5xx) with descriptions",
		[]string{"http status all", "all http status", "http status list"},
	)}
}
func (s *HTTPStatusAllSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "http status all") ||
		strings.Contains(low, "all http status") ||
		strings.Contains(low, "http status list") ||
		strings.Contains(low, "list http status")
}
func (s *HTTPStatusAllSkill) Execute(_ context.Context, _ string) (string, error) {
	return httpStatusAllTable, nil
}

const httpStatusAllTable = `Code  Meaning
100   Continue
101   Switching Protocols
102   Processing
103   Early Hints

200   OK
201   Created
202   Accepted
203   Non-Authoritative Information
204   No Content
205   Reset Content
206   Partial Content
207   Multi-Status
208   Already Reported
226   IM Used

300   Multiple Choices
301   Moved Permanently
302   Found
303   See Other
304   Not Modified
305   Use Proxy
307   Temporary Redirect
308   Permanent Redirect

400   Bad Request
401   Unauthorized
402   Payment Required
403   Forbidden
404   Not Found
405   Method Not Allowed
406   Not Acceptable
407   Proxy Authentication Required
408   Request Timeout
409   Conflict
410   Gone
411   Length Required
412   Precondition Failed
413   Payload Too Large
414   URI Too Long
415   Unsupported Media Type
416   Range Not Satisfiable
417   Expectation Failed
418   I'm a Teapot
421   Misdirected Request
422   Unprocessable Entity
423   Locked
424   Failed Dependency
425   Too Early
426   Upgrade Required
428   Precondition Required
429   Too Many Requests
431   Request Header Fields Too Large
451   Unavailable For Legal Reasons

500   Internal Server Error
501   Not Implemented
502   Bad Gateway
503   Service Unavailable
504   Gateway Timeout
505   HTTP Version Not Supported
506   Variant Also Negotiates
507   Insufficient Storage
508   Loop Detected
510   Not Extended
511   Network Authentication Required`

// ---- Common MIME types ----

type MIMETypeCommonSkill struct{ *kyoci.BaseSkill }

func NewMIMETypeCommonSkill() *MIMETypeCommonSkill {
	return &MIMETypeCommonSkill{BaseSkill: kyoci.NewBaseSkill(
		"mime_type_common", "Return common MIME types and their file extensions",
		[]string{"mime type common", "common mime types", "mime type list"},
	)}
}
func (s *MIMETypeCommonSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "mime type common") ||
		strings.Contains(low, "common mime types") ||
		strings.Contains(low, "mime type list") ||
		strings.Contains(low, "list mime types")
}
func (s *MIMETypeCommonSkill) Execute(_ context.Context, _ string) (string, error) {
	return mimeTypeTable, nil
}

const mimeTypeTable = `Extension  MIME Type
.txt       text/plain
.html      text/html
.htm       text/html
.css       text/css
.csv       text/csv
.js        application/javascript
.mjs       application/javascript
.json      application/json
.xml       application/xml
.yaml      application/yaml
.yml       application/yaml
.md        text/markdown
.svg       image/svg+xml
.png       image/png
.jpg       image/jpeg
.jpeg      image/jpeg
.gif       image/gif
.webp      image/webp
.ico       image/x-icon
.bmp       image/bmp
.tiff      image/tiff
.tif       image/tiff
.pdf       application/pdf
.zip       application/zip
.gz        application/gzip
.tar       application/x-tar
.rar       application/vnd.rar
.7z        application/x-7z-compressed
.doc       application/msword
.docx      application/vnd.openxmlformats-officedocument.wordprocessingml.document
.xls       application/vnd.ms-excel
.xlsx      application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
.ppt       application/vnd.ms-powerpoint
.pptx      application/vnd.openxmlformats-officedocument.presentationml.presentation
.mp3       audio/mpeg
.wav       audio/wav
.ogg       audio/ogg
.mp4       video/mp4
.webm      video/webm
.avi       video/x-msvideo
.mkv       video/x-matroska
.mov       video/quicktime
.ttf       font/ttf
.otf       font/otf
.woff      font/woff
.woff2     font/woff2
.exe       application/octet-stream
.bin       application/octet-stream
.dmg       application/octet-stream
.iso       application/octet-stream`

// ---- Common HTML named entities ----

type HTMLEntityCommonSkill struct{ *kyoci.BaseSkill }

func NewHTMLEntityCommonSkill() *HTMLEntityCommonSkill {
	return &HTMLEntityCommonSkill{BaseSkill: kyoci.NewBaseSkill(
		"html_entity_common", "Return common HTML named character entities",
		[]string{"html entity", "html entity list"},
	)}
}
func (s *HTMLEntityCommonSkill) Match(q string) bool {
	low := strings.ToLower(q)
	// Match "html entity" or the plural "html entities" (which is NOT a substring
	// of "html entity" because of the y→ies shift).
	hasEntity := strings.Contains(low, "html entity") || strings.Contains(low, "html entities")
	if !hasEntity {
		return false
	}
	// Require one of the table/list markers so we don't collide with the
	// HTML escape / unescape skills.
	return strings.Contains(low, "list") ||
		strings.Contains(low, "common") ||
		strings.Contains(low, "entities") ||
		strings.Contains(low, "table")
}
func (s *HTMLEntityCommonSkill) Execute(_ context.Context, _ string) (string, error) {
	return htmlEntityTable, nil
}

const htmlEntityTable = `Entity   Character   Description
&amp;    &           Ampersand
&lt;     <           Less than
&gt;     >           Greater than
&quot;   "           Quotation mark
&apos;   '           Apostrophe
&nbsp;   (space)     Non-breaking space
&copy;   (c)         Copyright sign
&reg;    (R)         Registered sign
&trade;  (TM)        Trademark sign
&deg;    (deg)       Degree sign
&plusmn; +/-          Plus-minus sign
&times;  x           Multiplication sign
&divide; (div)        Division sign
&frac12; 1/2         Fraction one half
&frac14; 1/4         Fraction one quarter
&frac34; 3/4         Fraction three quarters
&sup2;   2           Superscript two
&sup3;   3           Superscript three
&micro;  (mu)        Micro sign
&para;   (P)         Pilcrow sign
&middot; (.)         Middle dot
&bull;   (o)         Bullet
&dagger; (dagger)    Dagger
&hellip; ...         Horizontal ellipsis
&permil; (per mille) Per mille sign
&euro;   (EUR)       Euro sign
&pound;  (GBP)       Pound sign
&yen;    (JPY)       Yen sign
&cent;   (c slash)   Cent sign
&sect;   (S)         Section sign
&laquo;  <<          Left-pointing double angle quote
&raquo;  >>          Right-pointing double angle quote
&mdash;  --          Em dash
&ndash;  -           En dash
&lsquo;  '           Left single quotation mark
&rsquo;  '           Right single quotation mark
&ldquo;  "           Left double quotation mark
&rdquo;  "           Right double quotation mark`

// ---- ASCII table ----

type ASCIITableSkill struct{ *kyoci.BaseSkill }

func NewASCIITableSkill() *ASCIITableSkill {
	return &ASCIITableSkill{BaseSkill: kyoci.NewBaseSkill(
		"ascii_table", "Return printable ASCII table (codes 32-127) with decimal/hex/octal/glyph",
		[]string{"ascii table", "ascii chart"},
	)}
}
func (s *ASCIITableSkill) Match(q string) bool {
	low := strings.ToLower(q)
	if !strings.Contains(low, "ascii") {
		return false
	}
	return strings.Contains(low, "table") || strings.Contains(low, "chart")
}
func (s *ASCIITableSkill) Execute(_ context.Context, _ string) (string, error) {
	return asciiTableString(), nil
}

// asciiTableString generates the printable-ASCII reference table at runtime
// from the code points 32..126. Returned format:
//
//	Dec  Hex  Oct  Glyph
//	32   20   040  (space)
//	33   21   041  !
//	...
//	126  7E   176  ~
//
// Generating rather than hand-writing avoids typos and keeps the file short.
func asciiTableString() string {
	var b strings.Builder
	b.WriteString("Dec  Hex  Oct   Glyph\n")
	b.WriteString("---  ---  ---   -----\n")
	for c := 32; c <= 126; c++ {
		glyph := string(rune(c))
		if c == 32 {
			glyph = "(space)"
		} else if c == 127 {
			glyph = "(DEL)"
		}
		b.WriteString(fmt.Sprintf("%-3d  %-3s  %-3s   %s\n",
			c,
			strconv.FormatInt(int64(c), 16),
			strconv.FormatInt(int64(c), 8),
			glyph,
		))
	}
	return strings.TrimRight(b.String(), "\n")
}

// ---- UUID namespaces (RFC 4122) ----

type UUIDNamespaceDNSSkill struct{ *kyoci.BaseSkill }

func NewUUIDNamespaceDNSSkill() *UUIDNamespaceDNSSkill {
	return &UUIDNamespaceDNSSkill{BaseSkill: kyoci.NewBaseSkill(
		"uuid_namespace_dns", "Return the RFC 4122 UUIDv5 namespace UUID for DNS",
		[]string{"uuid namespace dns", "namespace dns uuid"},
	)}
}
func (s *UUIDNamespaceDNSSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "uuid namespace dns") ||
		strings.Contains(low, "namespace dns uuid") ||
		strings.Contains(low, "dns namespace uuid")
}
func (s *UUIDNamespaceDNSSkill) Execute(_ context.Context, _ string) (string, error) {
	return "{6ba7b810-9dad-11d1-80b4-00c04fd430c8}", nil
}

type UUIDNamespaceURLSkill struct{ *kyoci.BaseSkill }

func NewUUIDNamespaceURLSkill() *UUIDNamespaceURLSkill {
	return &UUIDNamespaceURLSkill{BaseSkill: kyoci.NewBaseSkill(
		"uuid_namespace_url", "Return the RFC 4122 UUIDv5 namespace UUID for URL",
		[]string{"uuid namespace url", "namespace url uuid"},
	)}
}
func (s *UUIDNamespaceURLSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "uuid namespace url") ||
		strings.Contains(low, "namespace url uuid") ||
		strings.Contains(low, "url namespace uuid")
}
func (s *UUIDNamespaceURLSkill) Execute(_ context.Context, _ string) (string, error) {
	return "{6ba7b811-9dad-11d1-80b4-00c04fd430c8}", nil
}

type UUIDNamespaceOIDSkill struct{ *kyoci.BaseSkill }

func NewUUIDNamespaceOIDSkill() *UUIDNamespaceOIDSkill {
	return &UUIDNamespaceOIDSkill{BaseSkill: kyoci.NewBaseSkill(
		"uuid_namespace_oid", "Return the RFC 4122 UUIDv5 namespace UUID for OID",
		[]string{"uuid namespace oid", "namespace oid uuid"},
	)}
}
func (s *UUIDNamespaceOIDSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "uuid namespace oid") ||
		strings.Contains(low, "namespace oid uuid") ||
		strings.Contains(low, "oid namespace uuid")
}
func (s *UUIDNamespaceOIDSkill) Execute(_ context.Context, _ string) (string, error) {
	return "{6ba7b812-9dad-11d1-80b4-00c04fd430c8}", nil
}

type UUIDNamespaceX500Skill struct{ *kyoci.BaseSkill }

func NewUUIDNamespaceX500Skill() *UUIDNamespaceX500Skill {
	return &UUIDNamespaceX500Skill{BaseSkill: kyoci.NewBaseSkill(
		"uuid_namespace_x500", "Return the RFC 4122 UUIDv5 namespace UUID for X.500",
		[]string{"uuid namespace x500", "namespace x500 uuid"},
	)}
}
func (s *UUIDNamespaceX500Skill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "uuid namespace x500") ||
		strings.Contains(low, "namespace x500 uuid") ||
		strings.Contains(low, "x500 namespace uuid")
}
func (s *UUIDNamespaceX500Skill) Execute(_ context.Context, _ string) (string, error) {
	return "{6ba7b814-9dad-11d1-80b4-00c04fd430c8}", nil
}

// ---- Unix signals ----

type UnixSignalListSkill struct{ *kyoci.BaseSkill }

func NewUnixSignalListSkill() *UnixSignalListSkill {
	return &UnixSignalListSkill{BaseSkill: kyoci.NewBaseSkill(
		"unix_signal_list", "Return common POSIX/Unix signals with their numeric values",
		[]string{"unix signal", "signal list", "posix signals"},
	)}
}
func (s *UnixSignalListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "unix signal") ||
		strings.Contains(low, "signal list") ||
		strings.Contains(low, "posix signals") ||
		strings.Contains(low, "list signals")
}
func (s *UnixSignalListSkill) Execute(_ context.Context, _ string) (string, error) {
	return unixSignalTable, nil
}

const unixSignalTable = `Number  Name       Action   Description
1       SIGHUP     Terminate  Hang up detected on controlling terminal
2       SIGINT     Terminate  Interrupt from keyboard (Ctrl-C)
3       SIGQUIT    Core       Quit from keyboard (Ctrl-\)
4       SIGILL     Core       Illegal Instruction
5       SIGTRAP    Core       Trace/breakpoint trap
6       SIGABRT    Core       Abort signal from abort(3)
7       SIGBUS     Core       Bus error (bad memory access)
8       SIGFPE     Core       Floating-point exception
9       SIGKILL    Terminate  Kill signal (cannot be caught or ignored)
10      SIGUSR1    Terminate  User-defined signal 1
11      SIGSEGV    Core       Invalid memory reference (segfault)
12      SIGUSR2    Terminate  User-defined signal 2
13      SIGPIPE    Terminate  Broken pipe: write to pipe with no readers
14      SIGALRM    Terminate  Timer signal from alarm(2)
15      SIGTERM    Terminate  Termination signal (default kill)
16      SIGSTKFLT  Terminate  Stack fault on coprocessor
17      SIGCHLD    Ignore     Child stopped or terminated
18      SIGCONT    Continue   Continue if stopped
19      SIGSTOP    Stop       Stop process (cannot be caught or ignored)
20      SIGTSTP    Stop       Stop typed at terminal (Ctrl-Z)
21      SIGTTIN    Stop       Terminal input for background process
22      SIGTTOU    Stop       Terminal output for background process
23      SIGURG     Ignore     Urgent condition on socket
24      SIGXCPU    Core       CPU time limit exceeded
25      SIGXFSZ    Core       File size limit exceeded
26      SIGVTALRM  Terminate  Virtual alarm clock
27      SIGPROF    Terminate  Profiling timer expired
28      SIGWINCH   Ignore     Window resize signal
29      SIGIO      Terminate  I/O now possible (poll-able event)
30      SIGPWR     Terminate  Power failure
31      SIGSYS     Core       Bad system call (SVr4)`

// ---- File signatures / magic bytes ----

type FileSignatureListSkill struct{ *kyoci.BaseSkill }

func NewFileSignatureListSkill() *FileSignatureListSkill {
	return &FileSignatureListSkill{BaseSkill: kyoci.NewBaseSkill(
		"file_signature_list", "Return common file magic-byte signatures (hex)",
		[]string{"file signature", "magic bytes", "file magic"},
	)}
}
func (s *FileSignatureListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "file signature") ||
		strings.Contains(low, "magic bytes") ||
		strings.Contains(low, "file magic") ||
		strings.Contains(low, "magic numbers")
}
func (s *FileSignatureListSkill) Execute(_ context.Context, _ string) (string, error) {
	return fileSignatureTable, nil
}

const fileSignatureTable = `Magic Bytes (hex)   Extension  Format
89504E47            .png       PNG image
FFD8FF               .jpg       JPEG image
47494638             .gif       GIF image
424D                 .bmp       BMP image (BM)
49492A00             .tif       TIFF image (little-endian)
4D4D002A             .tif       TIFF image (big-endian)
25504446             .pdf       PDF document (%PDF)
504B0304             .zip       ZIP archive (PK)
504B0506             .zip       ZIP archive (empty)
504B0708             .zip       ZIP archive (spanned)
526172211A07         .rar       RAR archive (Rar!)
377ABC AF271C        .7z        7-Zip archive
1F8B                 .gz        GZIP
425A68               .bz2       BZIP2 (BZh)
FD377A585A00         .xz        XZ
7573746172           .tar       POSIX tar (ustar)
38425053             .psd       Adobe Photoshop
4D5A                 .exe       Windows PE / DOS executable (MZ)
7F454C46             .elf       ELF executable
CAFE BABE             .class     Java class file
D4C3B2A1             .pcap      Libpcap capture file
415649420            .avi       RIFF AVI
52494646             .wav/.avi  RIFF container (RIFF....)
664C6143             .flac      FLAC audio
494433               .mp3       MP3 audio (ID3)
1A45DFA3             .mkv/.webm EBML / Matroska / WebM
4F676753             .ogg/.ogv  Ogg container
25215053             .ps        PostScript (%!PS)
AC9EBD8F             .psd       Photoshop PSD (alt)
524D464F             .rmf       Real Media
7B5C727466           .rtf       Rich Text Format ({\rtf)
213C617263683E       .ar        Unix archive (!<arch>)
ED AB EE DB          .rpm       RPM package
213C61726368          .deb       Debian package (!<arch>)`

// ---- Emoji shortcodes ----

type EmojiShortcodeListSkill struct{ *kyoci.BaseSkill }

func NewEmojiShortcodeListSkill() *EmojiShortcodeListSkill {
	return &EmojiShortcodeListSkill{BaseSkill: kyoci.NewBaseSkill(
		"emoji_shortcode_list", "Return common emoji shortcodes and descriptions",
		[]string{"emoji shortcode", "shortcode list"},
	)}
}
func (s *EmojiShortcodeListSkill) Match(q string) bool {
	low := strings.ToLower(q)
	return strings.Contains(low, "emoji shortcode") ||
		strings.Contains(low, "shortcode list") ||
		strings.Contains(low, "list shortcodes") ||
		strings.Contains(low, "emoji shortcodes")
}
func (s *EmojiShortcodeListSkill) Execute(_ context.Context, _ string) (string, error) {
	return emojiShortcodeTable, nil
}

const emojiShortcodeTable = `Code              Emoji  Description
:smile:           :)     Smiling face with open mouth
:laughing:        :D     Grinning squinting face
:grin:            :-D    Grinning face
:wink:            ;)     Winking face
:blush:           :")    Smiling face with smiling eyes
:heart_eyes:      <3_eyes Smiling face with heart-eyes
:kissing_heart:   :*     Face throwing a kiss
:stuck_out_tongue: :P    Face with stuck-out tongue
:joy:             :'D    Face with tears of joy
:rofl:            ROFL   Rolling on the floor laughing
:sob:             :'(    Loudly crying face
:cry:             :,(    Crying face
:angry:           >:(    Angry face
:rage:            >=(    Pouting face
:thinking:        :-?    Thinking face
:neutral_face:    :-|    Neutral face
:expressionless:  :|     Expressionless face
:confused:        :-S    Confused face
:worried:         :-/    Worried face
:frowning:        :(     Frowning face with open mouth
:persevere:       >_<    Persevering face
:weary:           (;_;)  Weary face
:sleepy:          -_-    Sleepy face
:sunglasses:      B)     Smiling face with sunglasses
:nerd:            :B     Nerd face
:thumbsup:        +1     Thumbs up sign
:thumbsdown:      -1     Thumbs down sign
:ok_hand:         OK     OK hand sign
:victory:         V      Victory hand
:wave:            hi     Waving hand sign
:clap:            *clap* Clapping hands sign
:point_up:        ^      White up pointing index
:raised_hands:    \o/    Person raising both hands in celebration
:pray:            +o+    Person with folded hands
:heart:           <3     Heavy black heart
:broken_heart:    </3    Broken heart
:two_hearts:      <3<3   Two hearts
:sparkles:        *.*    Sparkles
:fire:            ~fire~ Fire
:100:             100    Hundred points symbol
:star:            *      White medium star
:sunny:           (sun)  Sun with face
:cloud:           (cloud) Cloud
:umbrella:        (rain)  Umbrella with rain drops
:coffee:          (mug)  Hot beverage
:beer:            (beer) Clinking beer mugs
:pizza:           (pizza) Slice of pizza
:cake:            (cake) Shortcake
:apple:           (apple) Red apple`

// =====================================================================================
// Helper: returns the list of skill constructor names for tooling / introspection.
// Not used at runtime by the registry; kept for parity with the other table files.
// =====================================================================================

// lookupTableSkillNames returns the canonical constructor-name list for the
// lookup-table skills, in declaration order. Used by tests to detect drift.
func lookupTableSkillNames() []string {
	return []string{
		"NewISOCountryAlpha2ListSkill",
		"NewISOCountryAlpha3ListSkill",
		"NewISOCurrencyListSkill",
		"NewISOLanguageAlpha2ListSkill",
		"NewHTTPStatusAllSkill",
		"NewMIMETypeCommonSkill",
		"NewHTMLEntityCommonSkill",
		"NewASCIITableSkill",
		"NewUUIDNamespaceDNSSkill",
		"NewUUIDNamespaceURLSkill",
		"NewUUIDNamespaceOIDSkill",
		"NewUUIDNamespaceX500Skill",
		"NewUnixSignalListSkill",
		"NewFileSignatureListSkill",
		"NewEmojiShortcodeListSkill",
	}
}

// _ keeps strconv reachable even if a future refactor removes the only caller
// (asciiTableString). Compile-time guarantee against import-cycle surprises.
var _ = strconv.Itoa
