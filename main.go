package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"strings"

	"golang.org/x/text/language"
	"google.golang.org/api/option"

	"log"
	"os"

	"cloud.google.com/go/translate"
	"github.com/0xAX/notificator"
	"github.com/arrufat/clipboard"
	"google.golang.org/genai"
)

// GTranslate groups the client and the context needed for translation
type GTranslate struct {
	nmtClient *translate.Client
	llmClient *genai.Client
	llmModel  string
	llmConfig *genai.GenerateContentConfig
	ctx       context.Context
}

func createClientWithKey(useLLM bool) (*GTranslate, error) {
	ctx := context.Background()
	if useLLM {
		client, err := genai.NewClient(ctx, &genai.ClientConfig{
			APIKey:  os.Getenv("GEMINI_APIKEY"),
			Backend: genai.BackendGeminiAPI,
		})
		if err != nil {
			log.Fatal(err)
		}
		cfg := &genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{{
					Text: "You are a language translator.\n" +
						"Whenever you receive a message, you will only respond with a translated version of the message.\n" +
						"The rules are as follows: if the message is in English, translate it into Korean, otherwise, translate it into English.\n" +
						"You should strive for accuracy on the meaning and not on a literal translation.\n" +
						"Remember: the output should only contain the translated message.",
				}},
			},
		}
		return &GTranslate{nmtClient: nil, llmClient: client, llmModel: "gemini-2.0-flash-lite", llmConfig: cfg, ctx: ctx}, err
	} else {
		client, err := translate.NewClient(ctx, option.WithAPIKey(os.Getenv("GOOGLE_TRANSLATE_APIKEY")))
		if err != nil {
			return nil, err
		}
		return &GTranslate{nmtClient: client, llmClient: nil, llmModel: "", llmConfig: nil, ctx: ctx}, err
	}
}

func (gt *GTranslate) useLLM() bool {
	return gt.llmClient != nil
}

func (gt *GTranslate) close() {
	if gt.nmtClient != nil {
		gt.nmtClient.Close()
	}
}

func (gt *GTranslate) translate(targetLang, text string) (string, error) {
	lang, err := language.Parse(targetLang)
	if err != nil {
		return "", err
	}
	trans := ""
	if gt.nmtClient != nil {
		resp, err := gt.nmtClient.Translate(gt.ctx, []string{text}, lang, &translate.Options{Model: "nmt", Format: "text"})
		if err != nil {
			return "", err
		}
		trans = resp[0].Text
	} else if gt.llmClient != nil {
		resp, err := gt.llmClient.Models.GenerateContent(gt.ctx, gt.llmModel, genai.Text(text), gt.llmConfig)
		if err != nil {
			return "", err
		}
		trans = resp.Text()
	} else {
		return "", err
	}
	return trans, err
}

func (gt *GTranslate) detect(text string) (string, error) {
	lang, err := gt.nmtClient.DetectLanguage(gt.ctx, []string{text})
	if err != nil {
		return "", err
	}
	return fmt.Sprint(lang[0][0].Language), err
}

func (gt *GTranslate) SupportedLanguages(targetLang string) error {
	lang, err := language.Parse(targetLang)
	if err != nil {
		return err
	}
	if gt.nmtClient != nil {
		resp, err := gt.nmtClient.SupportedLanguages(gt.ctx, lang)
		if err != nil {
			return err
		}
		for i, lang := range resp {
			fmt.Printf("%3d - %s: %s\n", i, lang.Tag, lang.Name)
		}
		return err
	}
	return errors.New("The nmtClient was not intialized")
}

func main() {
	known := flag.String("k", "en", "the language you already know")
	learn := flag.String("l", "ko", "the language you are learning")
	useLLM := flag.Bool("llm", false, "use an LLM for translation")
	separator := flag.String("sep", "", "separator between the original and the translated text,"+
		" using \">\" will prepend each translated line with \"> \" instead")
	list := flag.Bool("list", false, "list all possible language codes")
	flag.Parse()

	notify := notificator.New(notificator.Options{
		DefaultIcon: "/usr/share/icons/hicolor/scalable/apps/org.gnome.Settings-region-symbolic.svg",
		AppName:     "TClip",
	})

	if hasPrimary {
		setPrimary(true)
	}
	text, err := clipboard.ReadAll()
	if err != nil {
		notify.Push("Error reading the clipboard", err.Error(), "", notificator.UR_NORMAL)
		return
	}

	if text == "" {
		notify.Push("Error", "No text selected", "", notificator.UR_NORMAL)
		return
	}
	text = strings.ReplaceAll(text, "\n\n", "\n")
	log.Println("selected text:", text)

	gTrans, err := createClientWithKey(*useLLM)
	if err != nil {
		if *useLLM {
			log.Fatalf("%v\nMake sure you have set the GOOGLE_TRANSLATE_APIKEY environment variable", err)
		} else {
			log.Fatalf("%v\nMake sure you have set the GEMINI_APIKEY environment variable", err)
		}
	}
	defer gTrans.close()

	if *list {
		gTrans.SupportedLanguages(*known)
		return
	}

	var trans = ""
	var det = ""
	if gTrans.useLLM() {
		trans, err = gTrans.translate(*known, text)
		if err != nil {
			notify.Push("Error", "Unable to translate the language", "", notificator.UR_NORMAL)
			log.Fatal(err)
		}
		det = "with LLM"
	} else {
		det, err := gTrans.detect(text)
		if err != nil {
			notify.Push("Error", "Unable to detect the language", "", notificator.UR_NORMAL)
			log.Fatal(err)
		}
		log.Println("detected language:", det)

		if det == *known {
			trans, err = gTrans.translate(*learn, text)
			if err != nil {
				notify.Push("Error", "Unable to translate the language", "", notificator.UR_NORMAL)
				log.Fatal(err)
			}
		} else {
			trans, err = gTrans.translate(*known, text)
			if err != nil {
				notify.Push("Error", "Unable to translate the language", "", notificator.UR_NORMAL)
				log.Fatal(err)
			}
		}
		det = "from " + det
	}

	log.Println("translated text:", trans)

	if hasPrimary {
		setPrimary(false)
	}

	if *separator != "" {
		if *separator == ">" {
			lines := strings.Split(trans, "\n")
			trans = text
			for i, line := range lines {
				if line == "" && i == len(lines)-1 {
					continue
				}
				trans += "\n> " + line
			}
		} else {
			trans = text + "\n" + *separator + "\n" + trans
		}
		trans = strings.TrimRight(trans, "\n")
	}

	if err := clipboard.WriteAll(trans); err != nil {
		log.Fatal(err)
	}

	notify.Push(fmt.Sprintf("Translating %s: %s", det, text), trans, "", notificator.UR_NORMAL)
}
