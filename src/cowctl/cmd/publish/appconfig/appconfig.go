package appconfig

import (
	"cowlibrary/applications"
	"cowlibrary/constants"
	cowlibutils "cowlibrary/utils"
	"cowlibrary/vo"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"cowctl/utils"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Args: cobra.NoArgs,

		Use:   "application",
		Short: "publish application",
		Long:  "publish application",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runE(cmd)
		},
	}

	cmd.Flags().StringP("name", "n", "", "Set your app name")
	cmd.Flags().String("version", "", "version of the app.")
	cmd.Flags().String("config-path", "", "path for the configuration file.")
	cmd.Flags().String("language", "", "set the language for application")
	cmd.Flags().String("client-id", "", "Id which is generated by the user")
	cmd.Flags().String("client-secret", "", "Secret key which is generated by user")
	cmd.Flags().String("sub-domain", "", "where to publish? dev/partner. default partner ")
	cmd.Flags().String("host", "", "where to publish? (eg:dev.compliancecow.live) ")
	cmd.Flags().Bool("can-override", false, "rule already exists in the system")

	return cmd
}

func runE(cmd *cobra.Command) error {

	additionalInfo, err := utils.GetAdditionalInfoFromCmd(cmd)
	if err != nil {
		return err
	}

	namePointer := &vo.CowNamePointersVO{}

	if cmd.Flags().HasFlags() {

		if appNameFlag := cmd.Flags().Lookup("name"); appNameFlag != nil {
			namePointer.Name = appNameFlag.Value.String()

		}

		if versionFlag := cmd.Flags().Lookup("version"); versionFlag != nil {
			namePointer.Version = versionFlag.Value.String()
		}

		if languageFlag := cmd.Flags().Lookup("language"); languageFlag != nil {
			additionalInfo.Language = languageFlag.Value.String()
		}

		if currentFlag := cmd.Flags().Lookup("can-override"); currentFlag != nil && currentFlag.Changed {
			if flagValue := currentFlag.Value.String(); cowlibutils.IsNotEmpty(flagValue) {
				currentFlag.Value.Set("false")
				additionalInfo.CanOverride, _ = strconv.ParseBool(flagValue)
			}
		}

		additionalInfo.ClientID = utils.GetFlagValueAndResetFlag(cmd, "client-id", "")
		additionalInfo.ClientSecret = utils.GetFlagValueAndResetFlag(cmd, "client-secret", "")
		additionalInfo.SubDomain = utils.GetFlagValueAndResetFlag(cmd, "sub-domain", "")
		additionalInfo.Host = utils.GetFlagValueAndResetFlag(cmd, "host", "")

	}

	err = publishApplicationRecursively(namePointer, additionalInfo, "Primary")
	if err != nil {
		return err
	}

	return err
}

func publishApplicationRecursively(namePointer *vo.CowNamePointersVO, additionalInfo *vo.AdditionalInfo, appType string) error {
	defaultConfigPath := cowlibutils.IsDefaultConfigPath(constants.CowDataDefaultConfigFilePath)

	appDeclarativesPath := filepath.Join(additionalInfo.PolicyCowConfig.PathConfiguration.DeclarativePath, constants.UserDefinedApplicationPath)

	if cowlibutils.IsEmpty(namePointer.Name) {
		if !defaultConfigPath {
			return errors.New("Set the application name using the 'name' flag")
		}

		name, err := utils.GetValueAsFolderNameFromCmdPrompt("Select the Application :", true, appDeclarativesPath, utils.ValidateString)
		if err != nil {
			return fmt.Errorf("invalid app name. app name:%s,error:%v", namePointer.Name, err)
		}
		namePointer.Name = name
		if cowlibutils.IsEmpty(namePointer.Name) {
			return fmt.Errorf("application name cannot be empty")
		}

		appDeclarativesPath = filepath.Join(appDeclarativesPath, strings.ToLower(namePointer.Name))
		if cowlibutils.IsFolderNotExist(appDeclarativesPath) {
			return fmt.Errorf("application not available")
		}
	}

	linkedApplications, _ := applications.GetLinkedApplications(namePointer, additionalInfo)
	if len(linkedApplications) > 0 {
		linkedAppNames := make([]string, 0)
		for _, linkedApp := range linkedApplications {
			linkedAppNames = append(linkedAppNames, linkedApp.Name)
		}
		d := color.New(color.FgGreen, color.Bold)
		d.Printf("\nLinked application types found for '%s'. We are publishing these linked applications: %v\n", namePointer.Name, strings.Join(linkedAppNames, ","))
		for _, linkedApp := range linkedApplications {
			newNamePointer := &vo.CowNamePointersVO{
				Name:    linkedApp.Name,
				Version: linkedApp.Version,
			}
			newAdditionalInfo := &vo.AdditionalInfo{
				PolicyCowConfig: additionalInfo.PolicyCowConfig,
			}
			err := publishApplicationRecursively(newNamePointer, newAdditionalInfo, "Linked")
			if err != nil {
				return err
			}
		}
	}

	appPath := additionalInfo.PolicyCowConfig.PathConfiguration.AppConnectionPath
	packageName := strings.ToLower(namePointer.Name)
	if cowlibutils.IsFolderExist(filepath.Join(appPath, "go", packageName)) && cowlibutils.IsFolderExist(filepath.Join(appPath, "python", "appconnections", packageName)) {
		if cowlibutils.IsEmpty(additionalInfo.Language) {
			languageFromCmd, err := utils.GetConfirmationFromCmdPromptWithOptions(fmt.Sprintf("Two implementations have been found for the '%s' application class. Which language do you intend to publish it in? Python/Go (default: Go):", namePointer.Name), "go", []string{"go", "python"})
			if err != nil {
				return err
			}
			additionalInfo.Language = languageFromCmd
		}
	}
	d := color.New(color.FgMagenta, color.Italic)
	d.Printf("We are publishing the %v application: %v\n", appType, namePointer.Name)
	errorDetails := applications.PublishApplication(namePointer, additionalInfo)
	if len(errorDetails) > 0 {
		if strings.Contains(errorDetails[0].Issue, constants.ErrorAppAlreadyPresent) {
			if !defaultConfigPath && !additionalInfo.CanOverride {
				return errors.New("The application type is already present in the system. To override with a new implementation, set the 'can-override' flag as true")
			}
			isConfirmed, err := utils.GetConfirmationFromCmdPrompt("The application type is already present in the system, and it will be overridden with a new implementation. Do you want to go ahead?")
			if err != nil {
				return err
			}
			if !isConfirmed {
				return nil
			}
			additionalInfo.CanOverride = true
			errorDetails = applications.PublishApplication(namePointer, additionalInfo)
		}
		if len(errorDetails) > 0 {
			utils.DrawErrorTable(errorDetails)
			return errors.New(constants.ErroInvalidData)
		}
	}
	d = color.New(color.FgCyan, color.Bold)
	d.Println("Hurray!.. Application Configuration has been published on behalf of you")

	return nil
}