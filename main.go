package main

import (
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	goteamsnotify "github.com/atc0005/go-teams-notify/v2"
	corev2 "github.com/sensu/sensu-go/api/core/v2"
	sensu "github.com/sensu/sensu-plugin-sdk/sensu"
	templates "github.com/sensu/sensu-plugin-sdk/templates"
)

// HandlerConfig contains the Teams handler configuration
type HandlerConfig struct {
	sensu.PluginConfig
	teamswebHookURL          string
	teamsIsTest              string
	teamsSender              string
	teamsSensuURL            string
	teamsDescriptionTemplate string
	teamsMaxOutputLength     uint64
}

const (
	webHookURL          = "webhook-url"
	isTest              = "is-test"
	sender              = "sender"
	sensuURL            = "sensu-url"
	descriptionTemplate = "description-template"
	maxOutputLength     = "max-output-length"

	defaultIsTest          = "false"
	defaultSensuURL        = "http://localhost:3000"
	defaultSender          = "Sensu"
	defaultTemplate        = "{{ .Check.Output }}"
	defaultMaxOutputLength = uint64(20000)
	truncationMark         = "\n\n… truncated"
)

var (
	config = HandlerConfig{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-teams-handler",
			Short:    "The handler adapted from the Sensu Go Slack handler for notifying a Microsoft Teams channel",
			Keyspace: "sensu.io/plugins/teams/config",
		},
	}

	teamsConfigOptions = []*sensu.PluginConfigOption{
		{
			Path:      webHookURL,
			Env:       "TEAMS_WEBHOOK_URL",
			Argument:  webHookURL,
			Shorthand: "w",
			Secret:    true,
			Usage:     "The webhook url to send messages to",
			Value:     &config.teamswebHookURL,
		},
		{
			Path:      isTest,
			Env:       "TEAMS_IS_TEST",
			Argument:  isTest,
			Shorthand: "t",
			Default:   defaultIsTest,
			Usage:     "Specify if this is a test run",
			Value:     &config.teamsIsTest,
		},
		{
			Path:      sender,
			Env:       "TEAMS_SENDER",
			Argument:  sender,
			Shorthand: "s",
			Default:   defaultSender,
			Usage:     "The name that messages will be sent as",
			Value:     &config.teamsSender,
		},
		{
			Path:      sensuURL,
			Env:       "TEAMS_SENSU_URL",
			Argument:  sensuURL,
			Shorthand: "u",
			Default:   defaultSensuURL,
			Usage:     "A URL to an image to use as the user avatar",
			Value:     &config.teamsSensuURL,
		},
		{
			Path:      descriptionTemplate,
			Env:       "TEAMS_DESCRIPTION_TEMPLATE",
			Argument:  descriptionTemplate,
			Shorthand: "d",
			Default:   defaultTemplate,
			Usage:     "The Teams notification output template, in Golang text/template format",
			Value:     &config.teamsDescriptionTemplate,
		},
		{
			Path:      maxOutputLength,
			Env:       "TEAMS_MAX_OUTPUT_LENGTH",
			Argument:  maxOutputLength,
			Shorthand: "o",
			Default:   defaultMaxOutputLength,
			Usage:     "Max length of the message body (template output) in bytes; 0 disables truncation",
			Value:     &config.teamsMaxOutputLength,
		},
	}
)

func main() {
	goHandler := sensu.NewGoHandler(&config.PluginConfig, teamsConfigOptions, checkArgs, sendMessage)
	goHandler.Execute()
}

func checkArgs(_ *corev2.Event) error {
	if len(config.teamswebHookURL) == 0 {
		return fmt.Errorf("--%s or TEAMS_WEBHOOK_URL environment variable is required", webHookURL)
	}

	return nil
}

func formattedEventAction(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "RESOLVED"
	default:
		return "ALERT"
	}
}

func formattedTitle(event *corev2.Event) string {
	return fmt.Sprintf("%s - %s", config.teamsSender, formattedEventAction(event))
}

func chomp(s string) string {
	return strings.Trim(strings.Trim(strings.Trim(s, "\n"), "\r"), "\r\n")
}

func eventKey(event *corev2.Event) string {
	return fmt.Sprintf("%s/%s", event.Entity.Name, event.Check.Name)
}

func eventSummary(event *corev2.Event, maxLength int) string {
	output := chomp(event.Check.Output)
	if len(event.Check.Output) > maxLength {
		output = output[0:maxLength] + "..."
	}
	return fmt.Sprintf("%s:%s", eventKey(event), output)
}

func formattedMessage(event *corev2.Event) string {
	return fmt.Sprintf("%s - %s", formattedEventAction(event), eventSummary(event, 100))
}

func messageColor(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "00FF00"
	case 2:
		return "FF0000"
	default:
		return "FFFF00"
	}
}

func messageStatus(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "Resolved"
	case 2:
		return "Critical"
	default:
		return "Warning"
	}
}

// truncateOutput shortens s so its UTF-8 byte length is <= maxBytes, appending
// truncationMark when truncated. maxBytes 0 means no truncation.
func truncateOutput(s string, maxBytes uint64) string {
	if maxBytes == 0 {
		return s
	}
	max := int(maxBytes)
	if len(s) <= max {
		return s
	}
	markLen := len(truncationMark)
	if max <= markLen {
		// Degenerate config: keep as much of the mark as fits.
		if max <= 0 {
			return ""
		}
		return truncationMark[:max]
	}
	keep := max - markLen
	// Avoid splitting a multi-byte rune at the cut point.
	for keep > 0 && !utf8.RuneStart(s[keep]) {
		keep--
	}
	return s[:keep] + truncationMark
}

func messageSection(event *corev2.Event) *goteamsnotify.MessageCardSection {

	description, err := templates.EvalTemplate("description", config.teamsDescriptionTemplate, event)
	if err != nil {
		fmt.Printf("%s: Error processing template: %s", config.PluginConfig.Name, err)
	}
	description = strings.Replace(description, `\n`, "\n", -1)

	section := goteamsnotify.NewMessageCardSection()

	section.ActivityTitle = event.Check.Name

	if config.teamsIsTest == "false" {
		section.ActivitySubtitle = time.Now().Local().String()
	} else {
		section.ActivitySubtitle = "2021-11-17 02:00"
	}

	if err := section.AddFactFromKeyValue("Sender:", config.teamsSender); err != nil {
		fmt.Printf("%s: error adding sender fact: %s\n", config.PluginConfig.Name, err)
	}
	if err := section.AddFactFromKeyValue("Status:", messageStatus(event)); err != nil {
		fmt.Printf("%s: error adding status fact: %s\n", config.PluginConfig.Name, err)
	}
	if err := section.AddFactFromKeyValue("Entity:", event.Entity.Name); err != nil {
		fmt.Printf("%s: error adding entity fact: %s\n", config.PluginConfig.Name, err)
	}

	if config.teamsIsTest == "false" {
		section.Text = truncateOutput(description, config.teamsMaxOutputLength)
	} else {
		section.Text = "Test"
	}

	return section
}

func messageActionOpenURI(name string, targetURL string) *goteamsnotify.MessageCardPotentialAction {

	action, err := goteamsnotify.NewMessageCardPotentialAction(goteamsnotify.PotentialActionOpenURIType, name)

	if err != nil {
		log.Fatal("error encountered when creating new action:", err)
	}

	action.MessageCardPotentialActionOpenURI.Targets =
		[]goteamsnotify.MessageCardPotentialActionOpenURITarget{
			{
				OS:  "default",
				URI: targetURL,
			},
		}

	return action
}

func sendMessage(event *corev2.Event) error {
	hook := goteamsnotify.NewClient()

	hook.SkipWebhookURLValidationOnSend(true)

	err := hook.Send(config.teamswebHookURL, goteamsnotify.MessageCard{
		Type:             "MessageCard",
		Context:          "https://schema.org/extensions",
		Summary:          "Sensu alert card",
		Title:            formattedTitle(event),
		ThemeColor:       messageColor(event),
		Sections:         []*goteamsnotify.MessageCardSection{messageSection(event)},
		PotentialActions: []*goteamsnotify.MessageCardPotentialAction{messageActionOpenURI("View in Sensu", config.teamsSensuURL)},
	})
	if err != nil {
		return fmt.Errorf("failed to send Teams message: %v", err)
	}

	// FUTURE: send to AH
	fmt.Print("Notification sent to Teams channel\n")

	return nil
}
