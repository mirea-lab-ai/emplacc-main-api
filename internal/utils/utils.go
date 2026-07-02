package utils

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

func GetString(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func GetBool(b *bool) bool {
	if b != nil {
		return *b
	}
	return false
}

func GetTime(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Time{}
}

func GetInt(t *int) int {
	if t != nil {
		return *t
	}
	return 0
}

func GetInt64(t *int64) int64 {
	if t != nil {
		return *t
	}
	return 0
}

func GetInt16(t *int16) int16 {
	if t != nil {
		return *t
	}
	return 0
}

func GetInt8(t *int8) int8 {
	if t != nil {
		return *t
	}
	return 0
}

func GetUUIDString(u *uuid.UUID) string {
	if u != nil {
		return u.String()
	}
	return ""
}

func FormatTimeForExcel(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02.01.2006")
}

func ConvertPriorityToText(priority int16) string {
	switch priority {
	case 5:
		return "Срочный"
	case 4:
		return "Высокий"
	case 3:
		return "Средний"
	case 2:
		return "Низкий"
	case 1:
		return "Не задан"
	default:
		return fmt.Sprintf("Неизвестный (%d)", priority)
	}
}

func BoolPtr(b bool) *bool {
	return &b
}

func EnglishToRussianKeyboard(text string) string {
	replacements := map[rune]rune{
		'q': 'й', 'w': 'ц', 'e': 'у', 'r': 'к', 't': 'е', 'y': 'н', 'u': 'г', 'i': 'ш', 'o': 'щ', 'p': 'з',
		'[': 'х', ']': 'ъ', 'a': 'ф', 's': 'ы', 'd': 'в', 'f': 'а', 'g': 'п', 'h': 'р', 'j': 'о', 'k': 'л',
		'l': 'д', ';': 'ж', '\'': 'э', 'z': 'я', 'x': 'ч', 'c': 'с', 'v': 'м', 'b': 'и', 'n': 'т', 'm': 'ь',
		',': 'б', '.': 'ю', '/': '.',

		'Q': 'Й', 'W': 'Ц', 'E': 'У', 'R': 'К', 'T': 'Е', 'Y': 'Н', 'U': 'Г', 'I': 'Ш', 'O': 'Щ', 'P': 'З',
		'{': 'Х', '}': 'Ъ', 'A': 'Ф', 'S': 'Ы', 'D': 'В', 'F': 'А', 'G': 'П', 'H': 'Р', 'J': 'О', 'K': 'Л',
		'L': 'Д', ':': 'Ж', '"': 'Э', 'Z': 'Я', 'X': 'Ч', 'C': 'С', 'V': 'М', 'B': 'И', 'N': 'Т', 'M': 'Ь',
		'<': 'Б', '>': 'Ю', '?': ',',
	}

	var result strings.Builder
	for _, char := range text {
		if replacement, exists := replacements[char]; exists {
			result.WriteRune(replacement)
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func RussianToEnglishKeyboard(text string) string {
	replacements := map[rune]rune{
		'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p',
		'х': '[', 'ъ': ']', 'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k',
		'д': 'l', 'ж': ';', 'э': '\'', 'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm',
		'б': ',', 'ю': '.', '.': '/',

		'Й': 'Q', 'Ц': 'W', 'У': 'E', 'К': 'R', 'Е': 'T', 'Н': 'Y', 'Г': 'U', 'Ш': 'I', 'Щ': 'O', 'З': 'P',
		'Х': '{', 'Ъ': '}', 'Ф': 'A', 'Ы': 'S', 'В': 'D', 'А': 'F', 'П': 'G', 'Р': 'H', 'О': 'J', 'Л': 'K',
		'Д': 'L', 'Ж': ':', 'Э': '"', 'Я': 'Z', 'Ч': 'X', 'С': 'C', 'М': 'V', 'И': 'B', 'Т': 'N', 'Ь': 'M',
		'Б': '<', 'Ю': '>', ',': '?',
	}

	var result strings.Builder
	for _, char := range text {
		if replacement, exists := replacements[char]; exists {
			result.WriteRune(replacement)
		} else {
			result.WriteRune(char)
		}
	}
	return result.String()
}

func PrepareTSQuery(query string) string {
	if query == "" {
		return ""
	}

	// Разбиваем на слова и обрабатываем каждое
	words := strings.Fields(query)
	processedWords := make([]string, 0, len(words))

	for _, word := range words {
		// Очищаем слово от лишних символов
		cleaned := cleanWord(word)
		if cleaned == "" {
			continue
		}

		// Добавляем поддержку prefix matching (:*)
		processedWords = append(processedWords, cleaned+":*")
	}

	if len(processedWords) == 0 {
		return ""
	}

	// Объединяем через AND (&)
	return strings.Join(processedWords, " & ")
}

// cleanWord очищает слово для полнотекстового поиска
func cleanWord(word string) string {
	// Приводим к нижнему регистру
	word = strings.ToLower(word)

	// Удаляем лишние символы, оставляем только буквы, цифры и дефисы
	var result strings.Builder
	for _, r := range word {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// PrepareSearchVariants создает варианты для поиска с учетом раскладки
func PrepareSearchVariants(query string) []string {
	if query == "" {
		return nil
	}

	variants := []string{query}

	// Добавляем варианты раскладки только для текстовых запросов
	if isTextQuery(query) {
		variants = append(variants,
			EnglishToRussianKeyboard(query),
			RussianToEnglishKeyboard(query),
		)
	}

	return variants
}

func isTextQuery(query string) bool {
	for _, r := range query {
		if !unicode.IsLetter(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
