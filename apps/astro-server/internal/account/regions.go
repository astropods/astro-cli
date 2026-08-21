package account

type region struct {
	label string
	flag  string
}

var regions = map[string]region{
	"us-east-1":      {"US East (N. Virginia)", "🇺🇸"},
	"us-east-2":      {"US East (Ohio)", "🇺🇸"},
	"us-west-1":      {"US West (N. California)", "🇺🇸"},
	"us-west-2":      {"US West (Oregon)", "🇺🇸"},
	"ca-central-1":   {"Canada (Central)", "🇨🇦"},
	"sa-east-1":      {"South America (São Paulo)", "🇧🇷"},
	"eu-west-1":      {"Europe (Ireland)", "🇮🇪"},
	"eu-west-2":      {"Europe (London)", "🇬🇧"},
	"eu-west-3":      {"Europe (Paris)", "🇫🇷"},
	"eu-central-1":   {"Europe (Frankfurt)", "🇩🇪"},
	"eu-north-1":     {"Europe (Stockholm)", "🇸🇪"},
	"eu-south-1":     {"Europe (Milan)", "🇮🇹"},
	"ap-south-1":     {"Asia Pacific (Mumbai)", "🇮🇳"},
	"ap-southeast-1": {"Asia Pacific (Singapore)", "🇸🇬"},
	"ap-southeast-2": {"Asia Pacific (Sydney)", "🇦🇺"},
	"ap-northeast-1": {"Asia Pacific (Tokyo)", "🇯🇵"},
	"ap-northeast-2": {"Asia Pacific (Seoul)", "🇰🇷"},
	"ap-east-1":      {"Asia Pacific (Hong Kong)", "🇭🇰"},
	"me-south-1":     {"Middle East (Bahrain)", "🇧🇭"},
	"af-south-1":     {"Africa (Cape Town)", "🇿🇦"},
}

func RegionLabel(r string) string {
	if info, ok := regions[r]; ok {
		return info.label
	}
	return r
}

func RegionFlag(r string) string {
	return regions[r].flag
}
