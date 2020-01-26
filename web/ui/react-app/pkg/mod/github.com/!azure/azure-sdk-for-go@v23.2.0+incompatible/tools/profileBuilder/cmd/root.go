// +build go1.9

// Copyright 2018 Microsoft Corporation
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/tools/profileBuilder/model"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// The values that should be used to identify the name parameter
const (
	nameLongName    = "name"
	nameShortName   = "n"
	nameDefault     = "<randomly generated>"
	nameDescription = "The name that should be used to identify the profile."
)

const (
	outputLocationLongName    = "output-location"
	outputLocationShortName   = "o"
	outputLocationDescription = "The folder in which to output the generated profile."
)

var outputLocationDefault = model.DefaultOutputLocation()

var cfgFile string

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "profileBuilder",
	Short: "Creates virtualized packages to simplify multi-API Version applications.",
	Long: `A profile is a virtualized set of packages, which attempts to hide the
complexity of choosing API Versions from customers who don't need the
flexiblity of separating the version of the Azure SDK for Go they're employing
from the version of Azure services they are targeting.

"profileBuilder" does the heavy-lifting of creating those virtualized packages.
Each of the sub-commands of profileBuilder applies a different strategy for
choosing which packages to include in the profile.
`,
	// Uncomment the following line if your bare application
	// has an action associated with it:
	//	Run: func(cmd *cobra.Command, args []string) { },
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.profileBuilder.yaml)")

	rootCmd.PersistentFlags().BoolP("clear-output", "c", false, "Removes any directories in the output-folder before writing a profile.")
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Use stderr to log verbose output.")

	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("clear-output", rootCmd.PersistentFlags().Lookup("clear-output"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// Search config in home directory with name ".profileBuilder" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".profileBuilder")
	}

	viper.AutomaticEnv() // read in environment variables that match

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}
}
