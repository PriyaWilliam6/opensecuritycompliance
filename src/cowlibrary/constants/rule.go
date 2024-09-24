package constants

var RuleMainFileData = `// This file is autogenerated. Please do not modify
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dmnlk/stringUtils"
	"github.com/fatih/color"
	"gopkg.in/yaml.v3"
)

func main() {

	if _, err := os.Stat("rule.json"); errors.Is(err, fs.ErrNotExist) {
		writeErrorMessageWithStr("rule.json is missing")
	} else {
		byts, err := os.ReadFile("rule.json")

		if err != nil {
			writeErrorMessage(err)
		} else {
			ruleSet := &RuleSet{}
			err = json.Unmarshal(byts, ruleSet)
			if err != nil {
				writeErrorMessage(err)
			}

			for _, rule := range ruleSet.Rules {

				taskInfos := make([]*TaskInfo, 0)

				byts, err := json.Marshal(rule.TasksInfo)
				if err != nil {
					writeErrorMessage(err)
				}

				err = json.Unmarshal(byts, &taskInfos)

				if err != nil {
					writeErrorMessage(err)
				}

				sort.Slice(taskInfos[:], func(i, j int) bool {
					return taskInfos[i].SeqNo < taskInfos[j].SeqNo
				})

				taskFolderNames := make([]string, 0)

				ruleProgressVO := &RuleProgressVO{}
				ruleProgressVO.Status = "INPROGRESS"
				ruleProgressVO.StartDateTime = time.Now()
				ruleProgressVO.Progress = make([]*TaskProgressVO, 0)

				defer func(ruleProgressVO *RuleProgressVO) {
					ruleProgressVO.EndDateTime = time.Now()
					ruleProgressVO.Duration = ruleProgressVO.EndDateTime.Sub(ruleProgressVO.StartDateTime)
					writeProgressFile(ruleProgressVO)
				}(ruleProgressVO)

				yamlInputByts, err := os.ReadFile("inputs.yaml")
				yamlTaskInputMap := make(map[string]interface{}, 0)
				if err == nil {
					yaml.Unmarshal(yamlInputByts, &yamlTaskInputMap)
					if val, ok := yamlTaskInputMap["userInputs"]; ok {

						if userInputsMap, ok := val.(map[interface{}]interface{}); ok {
							userInputsMapIn := make(map[string]interface{}, 0)
							for key, value := range userInputsMap {
								keyAsStr, ok := key.(string)
								if ok {
									userInputsMapIn[keyAsStr] = value
								}
							}
							rule.RuleIOValues.Inputs = userInputsMapIn
						}

						userInputsMap, ok := val.(map[string]interface{})
						if ok {
							rule.RuleIOValues.Inputs = userInputsMap
						}

						oldInputByts, err := json.Marshal(val)
						if err == nil {
							if rule.RuleIOValues == nil {
								rule.RuleIOValues = &IOValues{}
							}
							json.Unmarshal(oldInputByts, &rule.RuleIOValues.Inputs)
						}

					}
				}

				getOutputMapFromOutputFile := func() map[string]interface{} {
					if outputByts, err := os.ReadFile("output.json"); err == nil {
						outputMap := make(map[string]interface{}, 0)
						if err = json.Unmarshal(outputByts, &outputMap); err == nil {
							return outputMap
						}
					}
					return nil
				}

				getErrorMsgFromOutputFile := func() string {
					outputMap := getOutputMapFromOutputFile()
					if len(outputMap) > 0 {
						if errorMsg, ok := outputMap["error"]; ok {
							if errorMsgStr, ok := errorMsg.(string); ok {
								return errorMsgStr
							}

						}

					}
					return ""
				}

				ruleProgressVO.Inputs = rule.RuleIOValues.Inputs

				writeProgressFile(ruleProgressVO)

				for _, task := range taskInfos {
					taskName := strings.ReplaceAll(task.TaskGUID, "{{", "")
					taskName = strings.ReplaceAll(taskName, "}}", "")
					// fmt.Println("taskName :::", taskName)

					err := inputHandler(task, taskInfos, rule.RefMaps, rule.RuleIOValues, ruleSet)
					if err != nil {
						writeErrorMessage(err)
					}

					inputYamlByts, _ := os.ReadFile("inputs.yaml")

					appendTaskProgress := func(taskProgressVO TaskProgressVO, ruleProgressVO *RuleProgressVO) {
						clonedObj := taskProgressVO
						ruleProgressVO.Progress = append(ruleProgressVO.Progress, &clonedObj)
						writeProgressFile(ruleProgressVO)
					}

					taskProgressVO := TaskProgressVO{Name: taskName}

					if len(inputYamlByts) > 0 {

						taskInputPath := filepath.Join(taskName, "inputs.yaml")

						if _, err := os.Stat(taskInputPath); os.IsNotExist(err) {
							os.WriteFile(taskInputPath, inputYamlByts, os.ModePerm)
						}else {
                            inputYamlByts, _ = os.ReadFile(taskInputPath)
                        }

					}

					inputMap := make(map[string]interface{}, 0)

					err = yaml.Unmarshal(inputYamlByts, inputMap)
					if err == nil {
						if userInputs, ok := inputMap["userInputs"]; ok {

							if userInputsMap, ok := userInputs.(map[interface{}]interface{}); ok {
								userInputsMapIn := make(map[string]interface{}, 0)
								for key, value := range userInputsMap {
									keyAsStr, ok := key.(string)
									if ok {
										userInputsMapIn[keyAsStr] = value
									}
								}
								taskProgressVO.Inputs = userInputsMapIn
							}

							userInputsMap, ok := userInputs.(map[string]interface{})
							if ok {
								taskProgressVO.Inputs = userInputsMap
							}

						}
					}

					installPythonDependenciesWithRequirementsTxtFile(taskName)
					installPythonDependenciesWithRequirementsTxtFile(filepath.Join(taskName, "appconnections"))

					taskFolderNames = append(taskFolderNames, taskName)

					taskProgressVO.StartDateTime = time.Now()
					taskProgressVO.Status = "INPROGRESS"
					appendTaskProgress(taskProgressVO, ruleProgressVO)

					err = taskExecutor(taskName)

					taskProgressVO.EndDateTime = time.Now()
					taskProgressVO.Duration = taskProgressVO.EndDateTime.Sub(taskProgressVO.StartDateTime)

					taskProgressVO.Status = "COMPLETED"

					if taskProgressVO.Errors == nil {
						taskProgressVO.Errors = make(map[string]interface{}, 0)
					}

					if err != nil {
						taskProgressVO.Status = "ERROR"
						taskProgressVO.Errors = map[string]interface{}{
							"error": err,
						}
					}
					if outputByts, err := os.ReadFile(filepath.Join(taskName, "task_output.json")); err == nil {
						outputMap := make(map[string]interface{}, 0)
						if err = json.Unmarshal(outputByts, &outputMap); err == nil {
							if errorMsg, ok := outputMap["error"]; ok {
								taskProgressVO.Status = "ERROR"
								taskProgressVO.Errors["error"] = errorMsg
							} else if errorMsg, ok := outputMap["errors"]; ok {
								taskProgressVO.Status = "ERROR"
								taskProgressVO.Errors["error"] = errorMsg
							} else {
								taskProgressVO.Outputs, _ = outputMap["Outputs"].(map[string]interface{})
								// taskProgressVO.Outputs = ruleProgressVO.Outputs
							}
						}
					}

					if errorMsg := getErrorMsgFromOutputFile(); strings.TrimSpace(errorMsg) != "" {
						taskProgressVO.Status = "ERROR"
						taskProgressVO.Errors["error"] = errorMsg
					}

					if len(taskProgressVO.Errors) == 0 {
						taskProgressVO.Errors = nil
					}

					if ruleProgressVO.Errors == nil {
						ruleProgressVO.Errors = make(map[string]interface{}, 0)
					}

					appendTaskProgress(taskProgressVO, ruleProgressVO)

					if taskProgressVO.Status == "ERROR" {
						ruleProgressVO.Status = "ERROR"
						if len(taskProgressVO.Errors) > 0 {
							ruleProgressVO.Errors["taskName"] = taskName
							ruleProgressVO.Errors["error"] = taskProgressVO.Errors["error"]
						}
						return
					}

				}

				outputHandler(taskInfos, rule.RefMaps, taskFolderNames)

				ruleProgressVO.Status = "COMPLETED"

				if outputMap := getOutputMapFromOutputFile(); len(outputMap) > 0 {
					if errorMsg, ok := outputMap["error"]; ok {
						ruleProgressVO.Status = "ERROR"
						ruleProgressVO.Errors["error"] = errorMsg
					} else if errorMsg, ok := outputMap["errors"]; ok {
						ruleProgressVO.Status = "ERROR"
						ruleProgressVO.Errors["error"] = errorMsg
					} else {
						ruleProgressVO.Outputs = outputMap
					}
				}

				if len(ruleProgressVO.Errors) == 0 {
					ruleProgressVO.Errors = nil
				}

			}
		}
	}

}

func installPythonDependenciesWithRequirementsTxtFile(srcDir string) {
	if _, err := os.Stat(filepath.Join(srcDir, "requirements.txt")); err == nil {

		cmd := exec.Command("python3", "-m", "pip", "install", "-r", "requirements.txt")
		cmd.Dir = filepath.Join(srcDir)
		cmdByts, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Println("installation error :", err)
		} else {
			fmt.Println("installation output :", string(cmdByts))
		}

	}
}

func inputHandler(task *TaskInfo, taskInfos []*TaskInfo, refstruct []*RefStruct, ruleIOValues *IOValues, ruleSet *RuleSet) error {
	taskName := strings.ReplaceAll(task.TaskGUID, "{{", "")
	taskName = strings.ReplaceAll(taskName, "}}", "")

	inputMap := make(map[string]interface{})

	for _, ref := range refstruct {
		if ref.TargetRef.AliasRef == task.AliasRef {
			if ref.SourceRef.AliasRef == "*" {
				if ruleIOValues != nil && len(ruleIOValues.Inputs) > 0 {
					if val, ok := ruleIOValues.Inputs[ref.SourceRef.VarName]; ok {
						inputMap[ref.SourceRef.VarName] = val
					}
				}
			} else {
				outputs := make(map[string]map[string]interface{})
				for _, otherTask := range taskInfos {
					if otherTask.AliasRef == ref.SourceRef.AliasRef && stringUtils.IsNoneEmpty(ref.TargetRef.VarName) {
						otherTaskName := strings.ReplaceAll(otherTask.TaskGUID, "{{", "")
						otherTaskName = strings.ReplaceAll(otherTaskName, "}}", "")

						if _, ok := outputs[otherTaskName]; !ok {
							byts, err := os.ReadFile(otherTaskName + string(os.PathSeparator) + "task_output.json")
							if err != nil {
								return err
							}

							data := make(map[string]interface{})
							err = json.Unmarshal(byts, &data)
							if err != nil {
								return err
							}

							if val, ok := data["Outputs"]; ok {
								data, ok := val.(map[string]interface{})
								if !ok {
									return errors.New("no o/p available")
								}
								outputs[otherTaskName] = data
							}

						}

						if taskOutput, ok := outputs[otherTaskName]; ok {
							if val, newOk := taskOutput[ref.SourceRef.VarName]; newOk {
								inputMap[ref.TargetRef.VarName] = val
							}
						}

					}
				}
			}
		}
	}

	// fmt.Println("inputMap :", inputMap)
	// fmt.Println("taskName :", taskName)
	taskInput := &TaskInputs{}
	inputFileDir := taskName + string(os.PathSeparator) + "files" + string(os.PathSeparator)
	inputFilePath := inputFileDir + "TaskInputValue.json"
	yamlFlow := false
	if len(inputMap) > 0 {
		// fmt.Println("inputFilePath :", inputFilePath)

		oldInputs := make(map[string]interface{})
		if _, err := os.Stat(inputFileDir); os.IsNotExist(err) {
			if err := os.MkdirAll(inputFileDir, 0770); err != nil {
				return err
			}
		} else {

			if _, err := os.Stat(inputFilePath); os.IsExist(err) {
				byts, err := os.ReadFile(inputFilePath)
				if err != nil {
					return err
				}
				err = json.Unmarshal(byts, &oldInputs)
				if err != nil {
					return err
				}
			}

		}

		taskInputYamlPath := filepath.Join(taskName, "inputs.yaml")
		yamlInputByts, err := os.ReadFile(taskInputYamlPath)
		yamlTaskInputMap := make(map[string]interface{})
		if err == nil {
				err = yaml.Unmarshal(yamlInputByts, &yamlTaskInputMap)
				if val, ok := yamlTaskInputMap["userInputs"]; ok {
						oldInputByts, err := json.Marshal(val)
						if err == nil {
								err = json.Unmarshal(oldInputByts, &oldInputs)
								if err != nil {
										return err
								}
						}
				}
		}

		byts, err := json.Marshal(inputMap)
		if err != nil {
			return err
		}

		err = json.Unmarshal(byts, &oldInputs)
		if err != nil {
			return err
		}

		byts, err = json.Marshal(oldInputs)
		if err != nil {
			return err
		}

		err = os.WriteFile(inputFilePath, byts, os.ModePerm)
		if err != nil {
			return err
		}

		if len(yamlTaskInputMap) > 0 {
			yamlTaskInputMap["userInputs"] = oldInputs
			taskInputByts, err := yaml.Marshal(yamlTaskInputMap)
			if err == nil {
					err = os.WriteFile(taskInputYamlPath, taskInputByts, os.ModePerm)
					yamlFlow = true
			}
		}

		json.Unmarshal(byts, &(taskInput.UserInputs))

	}

	// if _, err := os.Stat(filepath.Join(taskName, "inputs.yaml")); err == nil {
	// 	byts, err := os.ReadFile(filepath.Join(taskName, "inputs.yaml"))
	// 	if err == nil {
	// 		yaml.Unmarshal(byts, &taskInput)
	// 	}
	// } else {
	readFromFileHelper(inputFileDir, "UserObjectServerValue", &(taskInput.UserObject))
	readFromFileHelper(inputFileDir, "UserObjectAppValue", &(taskInput.UserObject))
	readFromFileHelper(inputFileDir, "SystemObjectsValue", &(taskInput.SystemObjects))
	readFromFileHelper(inputFileDir, "MetaDataValue", &(taskInput.MetaData))
	readFromFileHelper(inputFileDir, "AdditionalInfos", &(taskInput.AdditionalInfos))
	// }

	if len(taskInput.UserInputs) == 0 {
		if _, err := os.Stat(inputFilePath); err == nil {
			byts, err := os.ReadFile(inputFilePath)
			if err == nil {
				json.Unmarshal(byts, &(taskInput.UserInputs))
			}
		}
	}

	if taskInput.MetaData!=nil && ruleSet!=nil{
		taskInput.MetaData.PlanExecutionGUID = ruleSet.PlanExecutionGUID
	}

	taskInputByts, err := json.Marshal(taskInput)
	if err != nil {
		return err
	}

	if !yamlFlow {

		taskInputFilePath := taskName + string(os.PathSeparator) + "task_input.json"
		taskInputByts =[]byte(os.ExpandEnv(string(taskInputByts)))
		err = os.WriteFile(taskInputFilePath, taskInputByts, os.ModePerm)
		if err != nil {
			return err
		}
	}

	return nil

}

func readFromFileHelper(inputFileDir, fileName string, target interface{}) {
	isTargetFilledWithData := false
	if _, err := os.Stat(inputFileDir); err == nil {
		if !strings.HasSuffix(inputFileDir, string(os.PathSeparator)) {
			inputFileDir += string(os.PathSeparator)
		}

		filePath := inputFileDir + fileName + ".json"
		if _, err := os.Stat(filePath); err == nil {
			byts, err := os.ReadFile(filePath)
			if err != nil {
				return
			}
			err = json.Unmarshal(byts, target)
			if err == nil {
				isTargetFilledWithData = true
			}
		}
	}

	if !isTargetFilledWithData {
		readFromFile(fileName, target)
	}

}

func readFromFile(fileName string, target interface{}) {
	readFileHelperWithExtension(fileName, "json", target)
}

func readFileHelperWithExtension(fileName, extension string, target interface{}) {
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	if !strings.HasSuffix(fileName, extension) {
		fileName += extension
	}
	readFileHelper(fileName, target, 4, 0)
}

func readFileHelper(fileName string, target interface{}, nestedLevelLimit int, count int) {
	if nestedLevelLimit < count {
		return
	}
	count++
	if !strings.Contains(fileName, string(os.PathSeparator)) {
		fileName = "files" + string(os.PathSeparator) + fileName
	} else {
		fileName = ".." + string(os.PathSeparator) + fileName
	}

	fs, err := os.Stat(fileName)
	if err != nil || fs.IsDir() {
		readFileHelper(fileName, target, nestedLevelLimit, count)
		return
	}

	byts, err := os.ReadFile(fileName)
	if err != nil {
		fmt.Println("error while reading " + fileName)
		return
	}
	json.Unmarshal(byts, target)
}

func outputHandler(taskInfos []*TaskInfo, refstruct []*RefStruct, taskNames []string) error {

	taskOutputs := make(map[string]map[string]interface{})

	for _, taskName := range taskNames {
		if byts, err := os.ReadFile(filepath.Join(taskName, "task_output.json")); err == nil && len(byts) > 2 {

			data := make(map[string]interface{})
			err = json.Unmarshal(byts, &data)
			if err != nil {
				return err
			}

			if val, ok := data["Outputs"]; ok {
				data, ok := val.(map[string]interface{})
				if !ok {
					return errors.New("no o/p available")
				}
				taskOutputs[taskName] = data
			}

		}

	}

	if taskOutputsByts, err := json.MarshalIndent(taskOutputs, "", "	"); err == nil {
		err = os.WriteFile("task_level_output.json", taskOutputsByts, os.ModePerm)
		if err != nil {
			writeErrorMessage(err)
		}
	}

	outputs := make(map[string]interface{})
	for _, ref := range refstruct {
		if ref.TargetRef.AliasRef == "*" && ref.TargetRef.FieldType == "Output" {
			for _, otherTask := range taskInfos {
				if otherTask.AliasRef == ref.SourceRef.AliasRef {
					otherTaskName := strings.ReplaceAll(otherTask.TaskGUID, "{{", "")
					otherTaskName = strings.ReplaceAll(otherTaskName, "}}", "")
					// if _, ok := taskOutputs[otherTaskName]; !ok {
					// 	byts, err := os.ReadFile(otherTaskName + string(os.PathSeparator) + "task_output.json")
					// 	if err != nil {
					// 		return err
					// 	}

					// }

					if taskOutput, ok := taskOutputs[otherTaskName]; ok {
						if val, newOk := taskOutput[ref.SourceRef.VarName]; newOk {
							outputs[ref.TargetRef.VarName] = val
						}
					}

				}

			}

		}
	}

	if len(outputs) > 0 {
		outputByts, err := json.MarshalIndent(outputs, "", "	")
		if err != nil {
			writeErrorMessage(err)
		}

		err = os.WriteFile("output.json", outputByts, os.ModePerm)
		if err != nil {
			writeErrorMessage(err)

		}

		if taskOutputsByts, err := json.MarshalIndent(taskOutputs, "", "	"); err == nil {
			err = os.WriteFile("task_level_output.json", taskOutputsByts, os.ModePerm)
			if err != nil {
				writeErrorMessage(err)
			}
		}

		color.Green(" %s \n", string(outputByts))
	}

	return nil

}

func writeErrorMessage(err error) {
	if err != nil {
		if stringUtils.IsNotEmpty(err.Error()) {
			writeErrorMessageWithStr(err.Error())
		}
	}
}

func writeErrorMessageWithStr(errStr string) {
	errorByts := []byte(` + "`" + `{"error":"` + "`+ errStr + `" + "\"}`" + `)
	err := os.WriteFile("output.json", errorByts, os.ModePerm)
	if err != nil {
		// TODO : handle the file write error
	}
}

func writeErrorMessageWithLogs(logs []byte, taskName string) {
	f, err := os.OpenFile("logs.txt", os.O_APPEND|os.O_WRONLY|os.O_CREATE, os.ModePerm)
	if err == nil {
		defer f.Close()

		f.WriteString("\n====" + taskName + "====\n")
		f.WriteString(string(logs))
	}	
}

func isPythonFlow(taskPath string) bool {
	if _, err := os.Stat(taskPath + "task.py"); os.IsNotExist(err) {
		return false
	}
	return true
}

func taskExecutor(taskName string) error{

	taskPath := taskName + string(os.PathSeparator)

	commandSeq := ""
	isPythonFlow := isPythonFlow(taskPath)
	if isPythonFlow {
		commandSeq += "autogenerated_main.py"
	} else {
		if _, err := os.Stat(taskPath + "go.mod"); os.IsNotExist(err) {
			commandSeq += "go mod init temp && "
		}

		//if _, err := os.Stat(taskPath + "go.sum"); os.IsNotExist(err) {
			commandSeq += "go mod tidy &&  "
		//}

		commandSeq += "go run *.go"
	}

	cmd := exec.Command("bash", "-c", commandSeq)
	if isPythonFlow {
		cmd = exec.Command("python3", "-u", "autogenerated_main.py")
		// cmd.Dir = curDir + "/" + taskName + "/"
	}
	curDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	taskDir := path.Join(curDir, taskName)
	cmd.Dir = taskDir

	outb, err := cmd.CombinedOutput()

	writeErrorMessageWithLogs(outb, taskName)

	if err != nil {
		writeErrorMessageWithStr("received an error from " + taskName + " execution. For a comprehensive overview at the rule level, review the 'logs.txt' file. If the task has started and the error remains unhandled, locate the 'logs.txt' inside the task. In the event that the error is handled within the task, refer to the 'task_output.json' file within the " + taskName + " folder for detailed error information")
	}

	outputByts, err := os.ReadFile(path.Join(taskDir, "task_output.json"))
	if err == nil {
		outputMap := make(map[string]interface{}, 0)
		err = json.Unmarshal(outputByts, &outputMap)
		if _, ok := outputMap["error"]; ok {
			return fmt.Errorf("Task execution failed for :%s", taskName)
		}
	}

	fmt.Println("")
	
	return nil

}

{{Task_VO}}

type TaskInputs struct {
	SystemInputs  ` + "`" + `yaml:",inline"` + "`" + `
	UserInputs map[string]interface{} ` + "`" + `yaml:"inputs"` + "`" + `
	AdditionalInfos AdditionalInfo
}

type PolicyCowConfig struct {
	Version           string                ` + "`" + `"json:"version" yaml:"version"` + "`" + `
	PathConfiguration *CowPathConfiguration ` + "`" + `json:"pathConfiguration" yaml:"pathConfiguration"` + "`" + `
}

type CowPathConfiguration struct {
	TasksPath     string ` + "`" + `json:"tasksPath" yaml:"tasksPath"` + "`" + `
	RulesPath     string ` + "`" + `json:"rulesPath" yaml:"rulesPath"` + "`" + `
	ExecutionPath string ` + "`" + `json:"executionPath" yaml:"executionPath"` + "`" + `
}

type AdditionalInfo struct {
	PolicyCowConfig *PolicyCowConfig ` + "`" + `json:"policyCowConfig" yaml:"policyCowConfig"` + "`" + `
	Path            string           ` + "`" + `json:"path" yaml:"path"` + "`" + `
	RuleName        string           ` + "`" + `json:"ruleName" yaml:"path"` + "`" + `
	ExecutionID     string           ` + "`" + `json:"executionID" yaml:"executionID"` + "`" + `
	RuleExecutionID string           ` + "`" + `json:"ruleExecutionID" yaml:"ruleExecutionID"` + "`" + `
	TaskExecutionID string           ` + "`" + `json:"taskExecutionID" yaml:"taskExecutionID"` + "`" + `
}

type SystemInputs struct {
	UserObject    *UserObjectTemplate ` + "`" + `yaml:"userObject"` + "`" + `
	SystemObjects []*ObjectTemplate ` + "`" + `yaml:"systemObjects"` + "`" + `
	MetaData      *MetaDataTemplate` + "`" + `yaml:"-"` + "`" + `
}
type UserObjectTemplate struct {
	ObjectTemplate          ` + "`" + `yaml:",inline"` + "`" + `
	Name                    string                       ` + "`" + `yaml:"name"` + "`" + `
	AppURL                  string                       ` + "`" + `yaml:"appURL"` + "`" + `
	Port                    int                           ` + "`" + `yaml:"appPort"` + "`" + `
	UserDefinedCredentialVO interface{} ` + "`" + `json:"userDefinedCredentials" yaml:"userDefinedCredentials"` + "`" + `
}
type ObjectTemplate struct {
	App         *AppAbstract  ` + "`" + `yaml:"app,omitempty"` + "`" + `
	Server      *ServerAbstract ` + "`" + `yaml:"server,omitempty"` + "`" + `
	Credentials []*Credential   ` + "`" + `yaml:"credentials,omitempty"` + "`" + `
}

type MetaDataTemplate struct {
	RuleGUID          string
	RuleTaskGUID      string
	ControlID         string
	PlanExecutionGUID string
}

type TaskOutputs struct {
	Outputs map[string]interface{}
}

type AppAbstract struct {
	*AppBase
	ID          string              ` + "`" + `json:"id,omitempty"` + "`" + `
	AppSequence int                 ` + "`" + `json:"appSequence,omitempty"` + "`" + `
	AppTags     map[string][]string ` + "`" + `json:"appTags,omitempty"` + "`" + `
	ActionType  string              ` + "`" + `json:"actionType,omitempty"` + "`" + `
	AppObjects  map[string]interface{}
	Servers     []*ServerAbstract ` + "`" + `json:"servers,omitempty"` + "`" + `
}

type AppBase struct {
	ApplicationName string ` + "`" + `json:"appName,omitempty"` + "`" + `
	ApplicationGUID string
	AppGroupGUID    string ` + "`" + `json:"appGroupId,omitempty"` + "`" + `
	ApplicationURL  string ` + "`" + `json:"appurl,omitempty"` + "`" + `
	OtherInfo       map[string]interface{}
}

type ServerBase struct {
	ServerGUID      string
	ServerName      string ` + "`" + `json:"servername,omitempty"` + "`" + `
	ApplicationGUID string ` + "`" + `json:"appid,omitempty"` + "`" + `
	ServerType      string ` + "`" + `json:"servertype,omitempty"` + "`" + `
	ServerURL       string ` + "`" + `json:"serverurl,omitempty"` + "`" + `
	ServerHostName  string ` + "`" + `json:"serverhostname,omitempty"` + "`" + `
}

type ServerAbstract struct {
	ServerBase
	ID            string              ` + "`" + `json:"id,omitempty"` + "`" + `
	ServerTags    map[string][]string ` + "`" + `json:"servertags,omitempty"` + "`" + `
	ServerBootSeq int                 ` + "`" + `json:"serverbootseq,omitempty"` + "`" + `
	ActionType    string              ` + "`" + `json:"actiontype,omitempty"` + "`" + `
	OSInfo        struct {
		OSDistribution string  ` + "`" + `json:"osdistribution,omitempty"` + "`" + `
		OSKernelLevel  string ` + "`" + `json:"oskernellevel,omitempty"` + "`" + `
		OSPatchLevel   string ` + "`" + `json:"ospatchlevel,omitempty"` + "`" + `
	} ` + "`" + `json:"osinfo,omitempty"` + "`" + `
	IPv4Addresses map[string]string ` + "`" + `json:"ipv4addresses,omitempty"` + "`" + `
	Volumes       map[string]string ` + "`" + `json:"volumes,omitempty"` + "`" + `
	OtherInfo     struct {
		CPU      int ` + "`" + `json:"cpu,omitempty"` + "`" + `
		GBMemory int ` + "`" + `json:"memory_gb,omitempty"` + "`" + `
	} ` + "`" + `json:"otherinfo,omitempty"` + "`" + `
	ClusterInfo struct {
		ClusterName    string            ` + "`" + `json:"clustername,omitempty"` + "`" + `
		ClusterMembers []*ServerAbstract ` + "`" + `json:"clustermembers,omitempty"` + "`" + `
	} ` + "`" + `json:"clusterinfo,omitempty"` + "`" + `
	Servers []*ServerAbstract ` + "`" + `json:"servers,omitempty"` + "`" + `
	// Services []*ServiceAbstract
}

// Credential : Holds Customer Credentials
type Credential struct {
	CredentialBase  ` + "`" + `yaml:",inline"` + "`" + `
	ID            string                 ` + "`" + `json:"id,omitempty" yaml:"id,omitempty"` + "`" + `
	PasswordHash  []byte                 ` + "`" + `json:"passwordhash,omitempty" yaml:"passwordhash,omitempty"` + "`" + `
	Password      string                 ` + "`" + `json:"passwordstring,omitempty" yaml:"password,omitempty"` + "`" + `
	LoginURL      string                 ` + "`" + `json:"loginurl,omitempty" yaml:"loginURL,omitempty" binding:"required,url" validate:"required,url"` + "`" + `
	SSHPrivateKey []byte                 ` + "`" + `json:"sshprivatekey,omitempty" yaml:"sshprivatekey,omitempty"` + "`" + `
	CredTags      map[string][]string    ` + "`" + `json:"credtags,omitempty" yaml:"tags,omitempty"` + "`" + `
	OtherCredInfo map[string]interface{} ` + "`" + `json:"othercredinfomap,omitempty" yaml:"otherCredentials" binding:"required" validate:"required"` + "`" + `
}


type CredentialBase struct {
    CredGUID   string ` + "`" + `json:"credguid,omitempty" yaml:"credguid,omitempty"` + "`" + `
    CredType   string ` + "`" + `json:"credtype,omitempty" yaml:"credType,omitempty"` + "`" + `
    SourceGUID string ` + "`" + `json:"sourceguid,omitempty" yaml:"sourceguid,omitempty"` + "`" + `
    SourceType string ` + "`" + `json:"sourcetype,omitempty" yaml:"sourcetype,omitempty"` + "`" + `
    UserID     string ` + "`" + `json:"userID,omitempty" yaml:"userid,omitempty"` + "`" + `
}



type RuleProgressVO struct {
	OutputVO
	Progress []*TaskProgressVO ` + "`" + `json:"progress,omitempty" yaml:"progress,omitempty"` + "`" + `
}

type OutputVO struct {
	Status        string                 ` + "`" + `json:"status,omitempty" yaml:"status,omitempty"` + "`" + `
	Outputs       map[string]interface{} ` + "`" + `json:"outputs,omitempty" yaml:"outputs,omitempty"` + "`" + `
	Inputs        map[string]interface{} ` + "`" + `json:"inputs,omitempty" yaml:"inputs,omitempty"` + "`" + `
	Errors        map[string]interface{} ` + "`" + `json:"errors,omitempty" yaml:"errors,omitempty"` + "`" + `
	StartDateTime time.Time              ` + "`" + `json:"startDateTime,omitempty" yaml:"startDateTime,omitempty"` + "`" + `
	EndDateTime   time.Time              ` + "`" + `json:"endDateTime,omitempty" yaml:"endDateTime,omitempty"` + "`" + `
	Duration      time.Duration          ` + "`" + `json:"duration,omitempty" yaml:"duration,omitempty"` + "`" + `
}

type TaskProgressVO struct {
	OutputVO
	Name string ` + "`" + `json:"name,omitempty" yaml:"name,omitempty"` + "`" + `
}

const ProgressFileName = "progress.json"

func writeProgressFile(ruleProgressVO *RuleProgressVO) {

	if progressByts, err := json.Marshal(ruleProgressVO); err == nil {
		os.WriteFile(ProgressFileName, progressByts, os.ModePerm)
	}

}





`

const TaskVO = "type RuleSet struct {\r\n\tId                string      `json:\"id,omitempty\"`\r\n\tRules             []*Rule     `json:\"rules,omitempty\"`\r\n\tHash              string      `json:\"hash,omitempty\"`\r\n\tType              string      `json:\"type,omitempty\"`\r\n\tApplicationscope  interface{} `json:\"applicationScope,omitempty\"`\r\n\tAppGroupGUID      string      `json:\"appGroupGUID,omitempty\"`\r\n\tPlanExecutionGUID string      `json:\"planExecutionGUID,omitempty\"`\r\n\tControlID         string      `json:\"controlID,omitempty\"`\r\n\tFromDate          time.Time   `json:\"fromDate,omitempty\"`\r\n\tToDate            time.Time   `json:\"toDate,omitempty\"`\r\n}\r\n\r\ntype RuleBase struct {\r\n\tRuleGUID      string `json:\"ruleGUID,omitempty\"`\r\n\tRuleName      string `json:\"rulename,omitempty\"`\r\n\tPurpose       string `json:\"purpose,omitempty\"`\r\n\tDescription   string `json:\"description,omitempty\"`\r\n\tAliasRef      string `json:\"aliasref,omitempty\"`\r\n\tSeqNo         int    `json:\"seqno,omitempty\"`\r\n\tInstanceName  string `json:\"instanceName,omitempty\"`\r\n\tObjectType    string `json:\"objectType,omitempty\"`\r\n\tObjectGUID    string `json:\"objectGUID,omitempty\"`\r\n\tRuleType      string `json:\"ruletype,omitempty\"`\r\n\tCoreHash      string `json:\"coreHash,omitempty\"`\r\n\tExtendedHash  string `json:\"extendedHash,omitempty\"`\r\n\tState         int    `json:\"-\"`\r\n\tCompliancePCT int    `json:\"compliancePCT,omitempty\"`\r\n}\r\n\r\ntype GeneralVO struct {\r\n\tDomainID string `json:\"domainId,omitempty\"`\r\n\tOrgID    string `json:\"orgId,omitempty\"`\r\n\tGroupID  string `json:\"groupId,omitempty\"`\r\n}\r\n\r\ntype Rule struct {\r\n\tRuleBase\r\n\tGeneralVO\r\n\tId                string              `json:\"id,omitempty\"`\r\n\tParentId          string              `json:\"parentId,omitempty\"`\r\n\tRootId            string              `json:\"rootId,omitempty\"`\r\n\tType              string              `json:\"type,omitempty\"`\r\n\tAppGroupGUID      string              `json:\"appGroupGUID,omitempty\"`\r\n\tPlanExecutionGUID string              `json:\"planExecutionGUID,omitempty\"`\r\n\tControlID         string              `json:\"controlID,omitempty\"`\r\n\tControlType       string              `json:\"controlType,omitempty\"`\r\n\tFromDate          time.Time           `json:\"fromDate,omitempty\"`\r\n\tToDate            time.Time           `json:\"toDate,omitempty\"`\r\n\tFailThresholdPCT  int                 `json:\"failThresholdPCT,omitempty\"`\r\n\tTasksInfo         []interface{}       `json:\"tasksinfo,omitempty\"`\r\n\tRuleIOValues      *IOValues           `json:\"ruleiovalues,omitempty\"`\r\n\tRefMaps           []*RefStruct        `json:\"refmaps,omitempty\"`\r\n\tRuleTags          map[string][]string `json:\"ruleTags,omitempty\"`\r\n\tRuleExceptions    []string            `json:\"-\"`\r\n\tMapOfMaps         map[string]string   `json:\"-\"`\r\n}\r\n\r\ntype TaskBase struct {\r\n\tTaskGUID    string `json:\"taskguid,omitempty\"`\r\n\tType        string `json:\"type,omitempty\"`\r\n\tAliasRef    string `json:\"aliasref,omitempty\"`\r\n\tPurpose     string `json:\"purpose,omitempty\"`\r\n\tRuleName    string `json:\"ruleName,omitempty\"`\r\n\tDescription string `json:\"description,omitempty\"`\r\n\tTaskState   int    `json:\"-\"`\r\n\tSeqNo       int    `json:\"seqno,omitempty\"`\r\n}\r\n\r\ntype TaskInfo struct {\r\n\tTaskBase\r\n\tId                string    `json:\"id,omitempty\"`\r\n\tAppGroupGUID      string    `json:\"appGroupGUID,omitempty\"`\r\n\tPlanExecutionGUID string    `json:\"planExecutionGUID,omitempty\"`\r\n\tControlID         string    `json:\"controlID,omitempty\"`\r\n\tRuleID            string    `json:\"ruleID,omitempty\"`\r\n\tObjectType        string    `json:\"objectType,omitempty\"`\r\n\tObjectGUID        string    `json:\"objectGUID,omitempty\"`\r\n\tFromDate          time.Time `json:\"fromDate,omitempty\"`\r\n\tToDate            time.Time `json:\"toDate,omitempty\"`\r\n\tFailThresholdPCT  int       `json:\"failThresholdPCT,omitempty\"`\r\n\tTaskIOValues      *IOValues `json:\"taskiovalues,omitempty\"`\r\n\tTaskException     error     `json:\"taskexception,omitempty\"`\r\n}\r\n\r\ntype RefStruct struct {\r\n\tTargetRef FieldMap `json:\"targetref,omitempty\"`\r\n\tSourceRef FieldMap `json:\"sourceref,omitempty\"`\r\n}\r\n\r\ntype FieldMap struct {\r\n\tFieldType string `json:\"fieldtype,omitempty\"`\r\n\tAliasRef  string `json:\"aliasref,omitempty\"`\r\n\tRuleName  string `json:\"ruleName,omitempty\"`\r\n\tTaskGUID  string `json:\"taskguid,omitempty\"`\r\n\tVarName   string `json:\"varname,omitempty\"`\r\n}\r\n\r\ntype IOValues struct {\r\n\tInputs          map[string]interface{} `json:\"inputs,omitempty\"`\r\n\tOutputs         map[string]interface{} `json:\"outputs,omitempty\"`\r\n\tFacts           map[string]interface{} `json:\"facts,omitempty\"`\r\n\tOutputFiles     map[string]string      `json:\"outputFiles,omitempty\"`\r\n\tDetailedInputs  []*DetailedInput       `json:\"inputs_,omitempty\"`\r\n\tDetailedOutputs map[string]interface{} `json:\"outputs_,omitempty\"`\r\n\tProcessFiles    []string               `json:\"processFiles,omitempty\"`\r\n\tTempFiles       []string               `json:\"tempFiles,omitempty\"`\r\n}\r\n\r\ntype DetailedInput struct {\r\n\tName              string            `json:\"name,omitempty\"`\r\n\tDisplay           string            `json:\"display,omitempty\"`\r\n\tType              string            `json:\"type,omitempty\"`\r\n\tMapper            string            `json:\"mapper,omitempty\"`\r\n\tIsMapper          bool              `json:\"ismapper,omitempty\"`\r\n\tIsResourcePattern bool              `json:\"isresourcepattern,omitempty\"`\r\n\tTemplate          string            `json:\"template,omitempty\"`\r\n\tIsRequired        bool              `json:\"isrequired,omitempty\"`\r\n\tShowFieldInUI     bool              `json:\"showfieldinui,omitempty\"`\r\n\tFormat            string            `json:\"format,omitempty\"`\r\n\tValue             string            `json:\"value,omitempty\"`\r\n\tDefaultValue      interface{}       `json:\"defaultvalue,omitempty\"`\r\n\tDescription       string            `json:\"description,omitempty\"`\r\n\tOutputFiles       map[string]string `json:\"outputFiles,omitempty\"`\r\n}\r\n\r\ntype RuleSetOutput struct {\r\n\tState            string        `json:\"state,omitempty\"`\r\n\tComplianceStatus string        `json:\"complianceStatus,omitempty\"`\r\n\tCompliancePCT    int           `json:\"compliancePCT,omitempty\"`\r\n\tType             string        `json:\"type,omitempty\"`\r\n\tRuleOutputs      []*RuleOutput `json:\"ruleOutputs,omitempty\"`\r\n}\r\n\r\ntype RuleOutput struct {\r\n\tOutputType       string         `json:\"outputType,omitempty\"`\r\n\tType             string         `json:\"type,omitempty\"`\r\n\tPurpose          string         `json:\"purpose,omitempty\"`\r\n\tDescription      string         `json:\"description,omitempty\"`\r\n\tAliasRef         string         `json:\"aliasref,omitempty\"`\r\n\tSeqNo            int            `json:\"seqno,omitempty\"`\r\n\tInstanceName     string         `json:\"instanceName,omitempty\"`\r\n\tObjectType       string         `json:\"objectType,omitempty\"`\r\n\tObjectGUID       string         `json:\"objectGUID,omitempty\"`\r\n\tState            string         `json:\"state,omitempty\"`\r\n\tComplianceStatus string         `json:\"complianceStatus,omitempty\"`\r\n\tCompliancePCT    int            `json:\"compliancePCT,omitempty\"`\r\n\tTaskState        map[string]int `json:\"taskState,omitempty\"`\r\n\tRuleIOValues     *IOValues      `json:\"ruleiovalues,omitempty\"`\r\n\tRuleOutputs      []*RuleOutput  `json:\"ruleOutputs,omitempty\"`\r\n}\r\n"

const RuleYAML = `apiVersion: v1alpha1
kind: rule
meta:
  name: RuleName
  purpose: Purpose of the Rule 
  description: Description about the rule
  labels:
    app:
spec:
  inputs:
    BucketName: demo
  inputsMeta__:
    - name: BucketName
      dataType: STRING
      repeated: false
      defaultValue: demo
      allowedValues: []
      showField: true
      required: true
  tasks:
    - alias: t1
      name: taskName
      purpose: Purpose of the task
      description: Detailed description about the task
  ioMap: 
    - 't1.Input.BucketName:=*.Input.BucketName'
    - '*.Output.CompliancePCT_:=t1.Output.CompliancePCT_'
    - '*.Output.ComplianceStatus_:=t1.Output.ComplianceStatus_'
    - '*.Output.LogFile:=t1.Output.LogFile'
`

const RuleJSON = `{
  "controlID": "ControlID",
  "planExecutionGUID": "PlanExecutionGUID",
  "appGroupGUID": "{{AppGroupGUID}}",
  "rules": [
    {
      "ruletags": {
        "tags": []
      },
      "Purpose": "Purpose of the rule",
      "Description": "Detailed info about the rule",
      "rulename": "Name of the rule",
      "ruleseqno": 1,
      "aliasref": "*",
      "ruletype": "sequential",
      "tasksinfo": [
        {
          "Purpose": "Purpose of the task",
          "Description": "Detailed info about the task",
          "type": "task",
          "aliasref": "t1",
          "taskguid": "{{Task1}}"
        }
      ],
      "ruleiovalues": {
        "inputs": {
          "key": "value"
        }
      },
      "refmaps": [
        {
          "sourceref": {
            "aliasref": "*",
            "fieldtype": "Input",
            "varname": "Key"
          },
          "targetref": {
            "aliasref": "t1",
            "fieldtype": "Input",
            "varname": "Key"
          }
        },
        {
          "sourceref": {
            "aliasref": "t1",
            "fieldtype": "Output",
            "varname": "CompliancePCT_"
          },
          "targetref": {
            "aliasref": "*",
            "fieldtype": "Output",
            "varname": "CompliancePCT_"
          }
        },
        {
          "sourceref": {
            "aliasref": "t1",
            "fieldtype": "Output",
            "varname": "ComplianceStatus_"
          },
          "targetref": {
            "aliasref": "*",
            "fieldtype": "Output",
            "varname": "ComplianceStatus_"
          }
        },
        {
          "sourceref": {
            "aliasref": "t1",
            "fieldtype": "Output",
            "varname": "LogFile"
          },
          "targetref": {
            "aliasref": "*",
            "fieldtype": "Output",
            "varname": "LogFile"
          }
        }
      ],
	  "evidences": []
    }
  ]
}
`
