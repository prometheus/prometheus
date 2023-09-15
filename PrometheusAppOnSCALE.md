---
title: "Installing Prometheus Using TrueNAS SCALE"
sort_rank:
---


{{< toc >}}

TrueNAS SCALE is an open source storage platform based on Debian Linux that is free to download and use. 
For information on TrueNAS SCALE click [here](https://www.truenas.com/truenas-scale/).

To download the latest public release of SCALE, click [here](https://www.truenas.com/download-truenas-scale/). 
For information on installing and using SCALE, see [Getting Started](https://www.truenas.com/docs/scale/gettingstarted/).  

TrueNAS SCALE makes installing Prometheus easy. SCALE deploys the Prometheus app in a Kubernetes container (pod).
SCALE installs Prometheus, completes the initial configuration, then starts the Prometheus Rule Manager. 
When updates become available, SCALE displays an update badge on the app on the **Installed Application** screen. 
To update to the latest release, click **Update** or **Update All** and SCALE takes care of the update process. 

Use the Prometheus web portal to configure HTTP endpoints (targets), labels, alerts, and queries.

## First Steps

Before installing the Prometheus app in SCALE, review [Prometheus Configuration documentation](https://prometheus.io/docs/prometheus/latest/configuration/configuration/) and the list of feature flags environment variables to see if you want to include any during the SCALE installation.
After deploying the app, you can edit the SCALE Prometheus app settings at any time to add environment variables or make changes to other settings. 

SCALE does not need advance preparation to install the Prometheus application. 
SCALE includes default settings for user and group, network port, retention time, and CPU/memory. 
You can install the app without changing default and other setting, or you can customize to suit the use case. 

Other than identifying Prometheus arguments or environment variables to add during installation, you can add a new user and group or the required datasets in SCALE. 

If you do not want to use the default (568) you can create a new user and group to manage the application. 
If [creating a new user (and group)](https://www.truenas.com/docs/scale/scaletutorials/credentials/managelocalusersscale/), take note of the user and group IDs.

SCALE can automatically create the two datasets Prometheus requires during app installation or you can create these datasets before deploying the application. 
If [creating the datasets](https://www.truenas.com/docs/scale/scaletutorials/storage/datasets/datasetsscale/) give them the names **data** and **config**. 

## Installing the Prometheus Application

To install the **Prometheus** application in SCALE, sign into SCALE, then go to **Apps**, click **Discover Apps**.
To locate the application, either begin typing Prometheus into the search field or scroll down to the **Prometheus** application widget.

![SCALE Prometheus App Widget](static/tutorial/SCALEPrometheusWidget.png)

Click on the widget to open the **Prometheus** application details screen.

![SCALE Prometheus App Details Screen](static/tutorial/SCALEPrometheusAppDetailsScreen.png)

Click **Install** to open the Prometheus app configuration screen.

The **Install Prometheus** screen groups settings by type. The sections below provide details on each.
To find specific fields click in the **Search Input Fields** search field and scroll down to it. 
To move to a particular section, click on the section heading on the navigation area in the upper-right corner.

![SCALE Install Prometheus Screen](static/tutorial/SCALEInstallPrometheusScreen.png)

Accept the default values in **Application Name** and **Version**. 

Accept the default value in **Retention Time** or change to suit your needs. 
Enter values in days (d), weeks (w), months (m), or years (y). For example, 15d, 2w, 3m, 1y. 

Enter the amount of storage space to allocate for the application in **Retention Size**. 
Valid entries include integer and suffix, for example: 100MB, 10GB, etc.

You can add arguments or environment variables to customize your installation but these are not required to deploy the app. 
To show the **Argument** entry field or the environment variable **Name** and **Value** fields, click **Add** for whichever type you want to add. 
Click again to add another argument or environment variable.

Accept the default port in **API Port**. 
Select **Host Network** to bind to the host network, but we recommend leaving this disabled.

Prometheus requires two storage datasets. 
You can allow SCALE to create these for you, or use the datasets named **data** and **config** created before in [First Steps](#first-steps).
Select the storage option you want to use for both **Prometheus Data Storage** and **Prometheus Config Storage**. 
Select **ixVolume** in **Type** to let SCALE create the dataset or select **Host Path** to use the existing datasets created on the system.

Accept the defaults in **Resource Configuration** or change the CPU and memory limits to suit your use case.

Click **Install**. 
The system opens the **Installed Applications** screen with the Prometheus app in the **Deploying** state.
When the installation completes it changes to **Running**. 

![SCALE Prometheus Installed](static/tutorial/SCALEPrometheusInstalled.png)

Click **Web Portal** on the **Application Info** widget to open the Prometheus web interface to begin configuring targets, alerts, rules and other parameters.

![Prometheus Web PortaL](static/tutorial/PrometheusWebPortal.png)

## Understanding Prometheus Settings
The following sections provide details on the settings found in the SCALE **Install Prometheus** screen.

### Application Name Settings

Accept the default value or enter a name in **Application Name**. 
Use the default name but if adding a second deployment of the application you must change this name.

Accept the default version number in **Version**. 
When a new version becomes available, the application has an update badge. 
The **Installed Applications** screen shows the option to update applications.

### Prometheus Configuration Settings

You can accept the defaults in the **Prometheus Configuration** settings or enter the settings you want to use.

![SCALE Prometheus Configuration Settings](static/tutorial/SCALEInstallPrometheusConfigSettings.png)

Accept the default in **Retention Time** or change to any value that suits your needs. 
Enter values in days (d), weeks (w), months (m), or years (y). For example, 15d, 2w, 3m, 1y. 

**Retention Size** is not required to install the application. To limit the space allocated to retain data add a value such as 100MB, 10GB, etc. 

Select **WAL Compression** to enable compressing the write-ahead log.

![SCALE Prometheus Configuration Add Argument and Environment Variable](static/tutorial/SCALEInstallPrometheusConfigAddArgEnvVar.png)

Add Prometheus environment variables in SCALE using the **Additional Environment Variables** option. 
Click **Add** for each variable you want to add.
Enter the Prometheus flag in **Name** and desired value in **Value**. For a complete list see Prometheus documentation on Feature Flags.

### Networking Settings

Accept the default port numbers in **API Port**.
The SCALE Prometheus app listens on port **30002**. 

Refer to the TrueNAS [default port list](https://www.truenas.com/docs/references/defaultports/) for a list of assigned port numbers.
To change the port numbers, enter a number within the range 9000-65535.

![SCALE Prometheus Network Configuration](static/tutorial/SCALEInstallPrometheusNetworkConfig.png)

We recommend not selecting **Host Network** as this binds to the host network.

### Storage Settings
You can install Prometheus using the default setting, ixVolume, or select the host path option and the two datasets created before installing the app.  

Both **Prometheus Data Storage** and **Pometheus Config Storage** default to **ixVolume (dataset created automatically by the system)**. This automatically creates the two required datasets.

![SCALE Prometheus Storage Configuration ixVolumes](static/tutorial/SCALEInstallPrometheusStorageConfigixVolume.png)

To use datasets created before beginning app installation, select **Host Path (Path that already exists on the system)** and browse to and select the **data** and **config** datasets.

![SCALE Prometheus Storage Configuration Host Paths](static/tutorial/SCALEInstallPrometheusStorageConfigHostPath.png)

Select **data** under **Prometheus Data Storage** and **config** under **Prometheus Config Storage**.

### Resource Configuration Settings

Accept the default values in **Resources Configuration** or enter new CPU and memory values.
By default, this application limits resources to no more than 4 CPU cores and 8 Gigabytes available memory. 
The application might use considerably less system resources.

![SCALE Prometheus Resource Configuration](static/tutorial/SCALEInstallPrometheusResourceConfig.png)

To customize the CPU and memory allocated to the container (pod) Prometheus uses, enter new CPU values as a plain integer value followed by the suffix m (milli). Default is 4000m.

Accept the default value 8Gi allocated memory or enter a new limit in bytes. 
Enter a plain integer followed by the measurement suffix, for example 129M or 123Mi.
