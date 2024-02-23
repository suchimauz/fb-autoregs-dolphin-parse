package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"gopkg.in/ini.v1"
)

var (
	nowTime    = time.Now()
	filePrefix = nowTime.Format("2006_01_02-15_04_05__")
)

func main() {
	config, err := ini.Load("settings.ini")
	if err != nil {
		fmt.Printf("Fail to read file: %v", err)
		os.Exit(1)
	}
	mainConfig := config.Section("main")
	parserConfig := config.Section("parser")

	autoregsFileName := mainConfig.Key("autoregs_file_name").String()
	outputDirectoryPath := mainConfig.Key("output_directory_path").String()
	templateFilePath := mainConfig.Key("template_file_path").String()

	if err := ensureDir(outputDirectoryPath); err != nil {
		fmt.Println("Directory creation failed with error: " + err.Error())
		os.Exit(1)
	}

	autoregsFile, err := os.ReadFile(autoregsFileName)
	if err != nil {
		log.Fatal(err)
	}
	autoregsContent := string(autoregsFile)

	if err = generateSplitAutoregsFile(autoregsContent, parserConfig, outputDirectoryPath); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	if err = generateDolphinMassImportFile(autoregsContent, parserConfig, templateFilePath, outputDirectoryPath); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func generateSplitAutoregsFile(content string, parserConfig *ini.Section, outputPath string) error {
	splitRegexpStr := parserConfig.Key("split_regex").String()
	splitRe := regexp.MustCompile(splitRegexpStr)
	autoregsContent := strings.ReplaceAll(content, "\t", "\n")
	splitted := splitRe.ReplaceAllString(autoregsContent, "$1\n")

	outputFile, err := os.OpenFile(filePath(outputPath, "splitted.txt"), os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return errors.New("Splitted file creation failed: " + err.Error())
	}

	_, err = outputFile.WriteString(splitted)
	if err != nil {
		return errors.New("Failed write to splitted file : " + err.Error())
	}

	outputFile.Close()

	return nil
}

func generateDolphinMassImportFile(content string, parserConfig *ini.Section, templateFilePath string, outputPath string) error {
	autoregsContent := strings.ReplaceAll(content, "\t", "\n")

	splitRegexpStr := parserConfig.Key("split_regex").String()
	profileNameRegexpStr := parserConfig.Key("profile_name_regex").String()
	cookieRegexpStr := parserConfig.Key("cookie_regex").String()
	userAgentRegexpStr := parserConfig.Key("user_agent_regex").String()

	splitRe := regexp.MustCompile(splitRegexpStr)
	profileNameRe := regexp.MustCompile(profileNameRegexpStr)
	cookieRe := regexp.MustCompile(cookieRegexpStr)
	userAgentRe := regexp.MustCompile(userAgentRegexpStr)

	filePath := filePath(outputPath, "dolphin.xlsx")
	err := CopyFile(templateFilePath, filePath)
	if err != nil {
		return errors.New("Copy template file failed: %s" + err.Error())
	}

	toSplitHack := "[hack_to_split]"
	toSplit := splitRe.ReplaceAllString(autoregsContent, "$1"+toSplitHack)
	var autoregs []string = strings.Split(toSplit, toSplitHack)

	file, err := excelize.OpenFile(filePath)
	if err != nil {
		return errors.New("Open dolphin file failed: %s" + err.Error())
	}

	startRow := 3
	row := startRow

	profileNameColumn := "A"
	cookieColumn := "B"
	userAgentColumn := "E"
	descriptionColumn := "F"

	for _, autoreg := range autoregs {
		if autoreg == "" {
			continue
		}

		var descriptionArr []string
		var profileName string
		var cookie string
		var userAgent string

		lines := strings.Split(autoreg, "\n")

		for _, l := range lines {
			if profileNameRe.MatchString(l) {
				profileName = l
			} else if cookieRe.MatchString(l) {
				cookie = l
			} else if userAgentRe.MatchString(l) {
				userAgent = l
				descriptionArr = append(descriptionArr, l)
			} else {
				descriptionArr = append(descriptionArr, l)
			}
		}
		description := strings.Join(descriptionArr, "\n")

		file.SetCellValue("Sheet1", fmt.Sprintf("%s%d", profileNameColumn, row), profileName)
		file.SetCellValue("Sheet1", fmt.Sprintf("%s%d", cookieColumn, row), cookie)
		file.SetCellValue("Sheet1", fmt.Sprintf("%s%d", userAgentColumn, row), userAgent)
		file.SetCellValue("Sheet1", fmt.Sprintf("%s%d", descriptionColumn, row), description)

		row++
	}

	file.Save()
	file.Close()

	return nil
}

func filePath(outputDirectoryPath string, name string) string {
	return fmt.Sprintf("%s/%s%s", outputDirectoryPath, filePrefix, name)
}

func ensureDir(dirName string) error {
	err := os.Mkdir(dirName, os.ModePerm)
	if err == nil {
		return nil
	}
	if os.IsExist(err) {
		// check that the existing path is a directory
		info, err := os.Stat(dirName)
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return errors.New("path exists but is not a directory")
		}
		return nil
	}
	return err
}

// CopyFile copies a file from src to dst. If src and dst files exist, and are
// the same, then return success. Otherise, attempt to create a hard link
// between the two files. If that fail, copy the file contents from src to dst.
func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return
	}
	if !sfi.Mode().IsRegular() {
		// cannot copy non-regular files (e.g., directories,
		// symlinks, devices, etc.)
		return fmt.Errorf("CopyFile: non-regular source file %s (%q)", sfi.Name(), sfi.Mode().String())
	}
	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return
		}
	} else {
		if !(dfi.Mode().IsRegular()) {
			return fmt.Errorf("CopyFile: non-regular destination file %s (%q)", dfi.Name(), dfi.Mode().String())
		}
		if os.SameFile(sfi, dfi) {
			return
		}
	}
	err = copyFileContents(src, dst)
	return
}

// copyFileContents copies the contents of the file named src to the file named
// by dst. The file will be created if it does not already exist. If the
// destination file exists, all it's contents will be replaced by the contents
// of the source file.
func copyFileContents(src, dst string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, os.ModePerm)
	if err != nil {
		return
	}
	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return
	}
	err = out.Sync()
	return
}
