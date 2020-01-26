package location

// Sample UN locodes.
var (
	SESTO UNLocode = "SESTO"
	AUMEL UNLocode = "AUMEL"
	CNHKG UNLocode = "CNHKG"
	USNYC UNLocode = "USNYC"
	USCHI UNLocode = "USCHI"
	JNTKO UNLocode = "JNTKO"
	DEHAM UNLocode = "DEHAM"
	NLRTM UNLocode = "NLRTM"
	FIHEL UNLocode = "FIHEL"
)

// Sample locations.
var (
	Stockholm = &Location{SESTO, "Stockholm"}
	Melbourne = &Location{AUMEL, "Melbourne"}
	Hongkong  = &Location{CNHKG, "Hongkong"}
	NewYork   = &Location{USNYC, "New York"}
	Chicago   = &Location{USCHI, "Chicago"}
	Tokyo     = &Location{JNTKO, "Tokyo"}
	Hamburg   = &Location{DEHAM, "Hamburg"}
	Rotterdam = &Location{NLRTM, "Rotterdam"}
	Helsinki  = &Location{FIHEL, "Helsinki"}
)
