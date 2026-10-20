package cmd

import "time"

type mcpClass string

const (
	mcpRead        mcpClass = "read"
	mcpWrite       mcpClass = "write"
	mcpDestructive mcpClass = "destructive"
	mcpExclude     mcpClass = "exclude"
)

type mcpPolicy struct {
	class mcpClass
	// args replaces the positional part of the command's Use line, in the same <required> [optional] syntax.
	args         string
	timeout      time.Duration
	excludeFlags []string
}

const (
	mcpDefaultTimeout = 2 * time.Minute
	mcpLongTimeout    = 15 * time.Minute
)

// mcpPolicies is keyed by command path without the binary name. Every visible leaf command needs an entry,
// except a top-level alias of a longer command.
var mcpPolicies = map[string]mcpPolicy{
	"account list":          {class: mcpRead},
	"agent get":             {class: mcpRead},
	"agent history":         {class: mcpRead},
	"agent list":            {class: mcpRead},
	"agent logs":            {class: mcpRead, excludeFlags: []string{"tail"}},
	"agent network":         {class: mcpRead},
	"agent trace":           {class: mcpRead},
	"agent usage":           {class: mcpRead},
	"billing agents":        {class: mcpRead},
	"billing get":           {class: mcpRead},
	"billing invoices":      {class: mcpRead},
	"billing models":        {class: mcpRead, args: "[model]"},
	"billing status":        {class: mcpRead},
	"blueprint builds get":  {class: mcpRead},
	"blueprint builds list": {class: mcpRead},
	"blueprint builds logs": {class: mcpRead, excludeFlags: []string{"tail"}},
	"blueprint get":         {class: mcpRead},
	"blueprint list":        {class: mcpRead},
	"dataset get":           {class: mcpRead},
	"dataset items":         {class: mcpRead},
	"dataset list":          {class: mcpRead},
	"env list":              {class: mcpRead},
	"eval get":              {class: mcpRead},
	"eval status":           {class: mcpRead},
	"eval validate":         {class: mcpRead},
	"gateway devices":       {class: mcpRead},
	"gateway status":        {class: mcpRead},
	"knowledge list":        {class: mcpRead},
	"knowledge status":      {class: mcpRead},
	"project list":          {class: mcpRead},
	"secrets get":           {class: mcpRead},
	"secrets list":          {class: mcpRead},
	"spec validate":         {class: mcpRead},
	"whoami":                {class: mcpRead},

	"agent pause":              {class: mcpWrite},
	"agent redeploy":           {class: mcpWrite, timeout: mcpLongTimeout},
	"agent restart":            {class: mcpWrite},
	"agent resume":             {class: mcpWrite},
	"agent trigger":            {class: mcpWrite},
	"billing set":              {class: mcpWrite},
	"blueprint build":          {class: mcpWrite, timeout: mcpLongTimeout},
	"blueprint builds rebuild": {class: mcpWrite, excludeFlags: []string{"tail"}},
	"blueprint deploy":         {class: mcpWrite, args: "<blueprint>", timeout: mcpLongTimeout, excludeFlags: []string{"secret"}},
	"blueprint push":           {class: mcpWrite, timeout: mcpLongTimeout},
	"blueprint set":            {class: mcpWrite},
	"dataset add":              {class: mcpWrite},
	"dataset edit":             {class: mcpWrite},
	"env create":               {class: mcpWrite},
	"env rename":               {class: mcpWrite},
	"eval push":                {class: mcpWrite},
	"eval review":              {class: mcpWrite},
	"eval run":                 {class: mcpWrite},
	"project trigger":          {class: mcpWrite, timeout: mcpLongTimeout},
	"secrets import":           {class: mcpWrite},

	"agent delete":      {class: mcpDestructive},
	"blueprint archive": {class: mcpDestructive},
	"dataset remove":    {class: mcpDestructive},
	"env delete":        {class: mcpDestructive},
	"gateway revoke":    {class: mcpDestructive},
	"knowledge delete":  {class: mcpDestructive},
	"secrets delete":    {class: mcpDestructive},

	"account switch":        {class: mcpExclude},
	"account token":         {class: mcpExclude},
	"dataset download":      {class: mcpExclude},
	"docs":                  {class: mcpExclude},
	"gateway connect":       {class: mcpExclude},
	"gateway disconnect":    {class: mcpExclude},
	"knowledge connect":     {class: mcpExclude},
	"knowledge credentials": {class: mcpExclude},
	"login":                 {class: mcpExclude},
	"logout":                {class: mcpExclude},
	"mcp":                   {class: mcpExclude},
	"project configure":     {class: mcpExclude},
	"project create":        {class: mcpExclude},
	"project logs":          {class: mcpExclude},
	"project start":         {class: mcpExclude},
	"project stop":          {class: mcpExclude},
	"secrets create":        {class: mcpExclude},
	"secrets update":        {class: mcpExclude},
	"settings bash":         {class: mcpExclude},
	"settings fish":         {class: mcpExclude},
	"settings powershell":   {class: mcpExclude},
	"settings update":       {class: mcpExclude},
	"settings zsh":          {class: mcpExclude},
	"upgrade":               {class: mcpExclude},
}
