/*
 * Copyright (c) 2018 DeineAgentur UG https://www.deineagentur.com. All rights reserved.
 * Licensed under the MIT License. See LICENSE file in the project root for full license information.
 */

package gotext

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/razor-1/localizer/store"
	"golang.org/x/text/language"
)

const (
	LCMessages = "LC_MESSAGES"
)

/*
Locale wraps the entire i18n collection for a single language (locale)
It's used by the package functions, but it can also be used independently to handle
multiple languages at the same time by working with this object.

Example:

    import (
	"encoding/gob"
	"bytes"
	    "fmt"
	    "github.com/leonelquinteros/gotext"
    )

    func main() {
        // Create Locale with library path and language code
        l := gotext.NewLocale("/path/to/i18n/dir", "en_US")

        // Load domain '/path/to/i18n/dir/en_US/LC_MESSAGES/default.{po,mo}'
        l.AddDomain("default")

        // Translate text from default domain
        fmt.Println(l.Get("Translate this"))

        // Load different domain ('/path/to/i18n/dir/en_US/LC_MESSAGES/extras.{po,mo}')
        l.AddDomain("extras")

        // Translate text from domain
        fmt.Println(l.GetD("extras", "Translate this"))
    }

*/
type Locale struct {
	// Path to locale files.
	path string

	// Language for this Locale
	lang string

	tag language.Tag

	// List of available Domains for this locale.
	Domains map[string]*Domain

	// First AddDomain is default Domain
	defaultDomain string

	// Sync Mutex
	sync.RWMutex
}

// NewLocale creates and initializes a new Locale object for a given language.
// It receives a path for the i18n .po/.mo files directory (p) and a language code to use (l).
func NewLocale(path, locale string) *Locale {
	processedLocale := SimplifiedLocale(locale)
	return &Locale{
		path:    path,
		lang:    processedLocale,
		tag:     language.Make(processedLocale),
		Domains: make(map[string]*Domain),
	}
}

//findExt finds a file for the specified domain and extension (typically either a .po or .mo file)
func (l *Locale) findExt(dom, ext string) string {
	base := dom + "." + ext
	underscoreLang := strings.Replace(l.tag.String(), "-", "_", -1)
	options := make([]string, 3, 8)
	options[0] = filepath.Join(l.path, l.lang, LCMessages, base)
	options[1] = filepath.Join(l.path, l.tag.String(), LCMessages, base)
	options[2] = filepath.Join(l.path, underscoreLang, LCMessages, base)

	if len(l.lang) > 2 {
		options = append(options, filepath.Join(l.path, l.lang[:2], LCMessages, base))
	}

	options = append(options, filepath.Join(l.path, l.lang, base))
	options = append(options, filepath.Join(l.path, l.tag.String(), base))
	options = append(options, filepath.Join(l.path, underscoreLang, base))

	if len(l.lang) > 2 {
		options = append(options, filepath.Join(l.path, l.lang[:2], base))
	}

	for _, filename := range options {
		if _, err := os.Stat(filename); err == nil {
			return filename
		}
	}

	return ""
}

func modTime(file string) (tm time.Time) {
	if file == "" {
		return
	}

	stat, err := os.Stat(file)
	if err == nil {
		tm = stat.ModTime()
	}

	return
}

//selectFile determines whether to read the .mo or .po file based on last modified time, and returns the file name
//it returns the empty string "" if no file can be found
func (l *Locale) selectFile(dom string) string {
	pofile := l.findExt(dom, "po")
	mofile := l.findExt(dom, "mo")

	poTime := modTime(pofile)
	moTime := modTime(mofile)

	//we want to use the .mo file. use it if it's present, unless the .po file is newer.
	if poTime.After(moTime) {
		return pofile
	} else if !moTime.IsZero() {
		return mofile
	}

	return ""
}

// AddDomain creates a new domain for a given locale object
// If the domain exists, it gets reloaded.
func (l *Locale) AddDomain(dom string) error {
	file := l.selectFile(dom)
	if file == "" {
		return errors.New("no mo or po file found")
	}
	gt, err := ParseFile(file)
	if err != nil {
		return err
	}

	l.Lock()
	defer l.Unlock()
	if l.Domains == nil {
		l.Domains = make(map[string]*Domain)
	}
	if l.defaultDomain == "" {
		l.defaultDomain = dom
	}
	l.Domains[dom] = gt.GetDomain()

	return nil
}

// AddTranslator takes a domain name and a Translator object to make it available in the Locale object.
func (l *Locale) AddFile(dom string, gt GettextFile) {
	l.Lock()

	if l.Domains == nil {
		l.Domains = make(map[string]*Domain)
	}
	if l.defaultDomain == "" {
		l.defaultDomain = dom
	}
	l.Domains[dom] = gt.GetDomain()

	l.Unlock()
}

// GetDomain is the domain getter for Locale configuration
func (l *Locale) GetDomain() string {
	l.RLock()
	dom := l.defaultDomain
	l.RUnlock()
	return dom
}

// SetDomain sets the name for the domain to be used.
func (l *Locale) SetDomain(dom string) {
	l.Lock()
	l.defaultDomain = dom
	l.Unlock()
}

//GetTranslations conforms us to the store.TranslationStore interface
func (l *Locale) GetTranslations(tag language.Tag) (lc store.LocaleCatalog, err error) {
	if l.tag != tag {
		err = fmt.Errorf("tags do not match: %v != %v", l.tag, tag)
		return
	}

	lc = store.NewLocaleCatalog(tag)
	lc.Path = l.path
	l.RLock()
	defer l.RUnlock()
	for _, translator := range l.Domains {
		for msgID, msg := range translator.GetAll() {
			lc.Translations[msgID] = msg
		}
	}

	return
}

// Get uses a domain "default" to return the corresponding Translation of a given string.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) Get(str string, vars ...interface{}) string {
	return l.GetD(l.GetDomain(), str, vars...)
}

// GetN retrieves the (N)th plural form of Translation for the given string in the "default" domain.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetN(str, plural string, n int, vars ...interface{}) string {
	return l.GetND(l.GetDomain(), str, plural, n, vars...)
}

// GetD returns the corresponding Translation in the given domain for the given string.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetD(dom, str string, vars ...interface{}) string {
	// Sync read
	l.RLock()
	defer l.RUnlock()

	if l.Domains != nil {
		if _, ok := l.Domains[dom]; ok {
			if l.Domains[dom] != nil {
				return l.Domains[dom].Get(str, vars...)
			}
		}
	}

	return Printf(str, vars...)
}

// GetND retrieves the (N)th plural form of Translation in the given domain for the given string.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetND(dom, str, plural string, n int, vars ...interface{}) string {
	// Sync read
	l.RLock()
	defer l.RUnlock()

	if l.Domains != nil {
		if _, ok := l.Domains[dom]; ok {
			if l.Domains[dom] != nil {
				return l.Domains[dom].GetN(str, plural, n, vars...)
			}
		}
	}

	// Use western default rule (plural > 1) to handle missing domain default result.
	if n == 1 {
		return Printf(str, vars...)
	}
	return Printf(plural, vars...)
}

// GetC uses a domain "default" to return the corresponding Translation of the given string in the given context.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetC(str, ctx string, vars ...interface{}) string {
	return l.GetDC(l.GetDomain(), str, ctx, vars...)
}

// GetNC retrieves the (N)th plural form of Translation for the given string in the given context in the "default" domain.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetNC(str, plural string, n int, ctx string, vars ...interface{}) string {
	return l.GetNDC(l.GetDomain(), str, plural, n, ctx, vars...)
}

// GetDC returns the corresponding Translation in the given domain for the given string in the given context.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetDC(dom, str, ctx string, vars ...interface{}) string {
	// Sync read
	l.RLock()
	defer l.RUnlock()

	if l.Domains != nil {
		if _, ok := l.Domains[dom]; ok {
			if l.Domains[dom] != nil {
				return l.Domains[dom].GetC(str, ctx, vars...)
			}
		}
	}

	return Printf(str, vars...)
}

// GetNDC retrieves the (N)th plural form of Translation in the given domain for the given string in the given context.
// Supports optional parameters (vars... interface{}) to be inserted on the formatted string using the fmt.Printf syntax.
func (l *Locale) GetNDC(dom, str, plural string, n int, ctx string, vars ...interface{}) string {
	// Sync read
	l.RLock()
	defer l.RUnlock()

	if l.Domains != nil {
		if _, ok := l.Domains[dom]; ok {
			if l.Domains[dom] != nil {
				return l.Domains[dom].GetNC(str, plural, n, ctx, vars...)
			}
		}
	}

	// Use western default rule (plural > 1) to handle missing domain default result.
	if n == 1 {
		return Printf(str, vars...)
	}
	return Printf(plural, vars...)
}

// LocaleEncoding is used as intermediary storage to encode Locale objects to Gob.
type LocaleEncoding struct {
	Path          string
	Lang          string
	Domains       map[string][]byte
	DefaultDomain string
}

// MarshalBinary implements encoding BinaryMarshaler interface
func (l *Locale) MarshalBinary() ([]byte, error) {
	obj := new(LocaleEncoding)
	obj.DefaultDomain = l.defaultDomain
	obj.Domains = make(map[string][]byte)
	for k, v := range l.Domains {
		var err error
		obj.Domains[k], err = v.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	obj.Lang = l.lang
	obj.Path = l.path

	var buff bytes.Buffer
	encoder := gob.NewEncoder(&buff)
	err := encoder.Encode(obj)

	return buff.Bytes(), err
}

// UnmarshalBinary implements encoding BinaryUnmarshaler interface
func (l *Locale) UnmarshalBinary(data []byte) error {
	buff := bytes.NewBuffer(data)
	obj := new(LocaleEncoding)

	decoder := gob.NewDecoder(buff)
	err := decoder.Decode(obj)
	if err != nil {
		return err
	}

	l.defaultDomain = obj.DefaultDomain
	l.lang = obj.Lang
	l.path = obj.Path

	// Decode Domains
	l.Domains = make(map[string]*Domain)
	for k, v := range obj.Domains {
		var tr TranslatorEncoding
		buff := bytes.NewBuffer(v)
		trDecoder := gob.NewDecoder(buff)
		err := trDecoder.Decode(&tr)
		if err != nil {
			return err
		}

		l.Domains[k] = tr.GetFile().GetDomain()
	}

	return nil
}
