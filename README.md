Cloudera Installation 

Requirement: 

Supported Operating Systems
Cloudera Manager supports the following operating systems:
•	RHEL-compatible
o	Red Hat Enterprise Linux and CentOS, 64-bit
	5.7
	6.4
	6.5
	6.5 in SE Linux mode
	6.6
	6.6 in SE Linux mode
	6.7
	7.1
o	Oracle Enterprise Linux with default kernel and Unbreakable Enterprise Kernel, 64-bit
	5.6 (UEK R2)
	6.4 (UEK R2)
	6.5 (UEK R2, UEK R3)
	6.6 (UEK R3)
	6.7
	7.1
   Important: Cloudera supports RHEL 7 with the following limitations:
o	Only RHEL 7.1 is supported. RHEL 7.0 is not supported.
o	Navigator Encrypt is not supported on RHEL 7.1.
•	SLES - SUSE Linux Enterprise Server 11, 64-bit. Service Pack 2 or later is required if Cloudera Manager is used to manage CDH 5, and Service Pack 1 or later is required if Cloudera Manager is used to manage CDH 4. If you follow Installation Path A - Automated Installation by Cloudera Manager, the Updates repository must be active to use the embedded PostgreSQL database. The SUSE Linux Enterprise Software Development Kit 11 SP1 is required on hosts running the Cloudera Manager Agents.
•	Debian - Wheezy (7.0 and 7.1), Squeeze (6.0) (deprecated), 64-bit
•	Ubuntu - Trusty (14.04), Precise (12.04), Lucid (10.04) (deprecated), 64-bit
   Note:
•	Debian Squeeze and Ubuntu Lucid are supported only for CDH 4.
•	Using the same version of the same operating system on all cluster hosts is strongly recommended.
Supported JDK Versions
The version of Oracle JDK supported by Cloudera Manager depends on the version of CDH that is being managed. The following table lists the JDK versions supported on a Cloudera Manager 5.5 cluster running the latest CDH 4 and CDH 5. For further information on supported JDK versions for previous versions of Cloudera Manager and CDH, see JDK Compatibility.
Cloudera Manager can install Oracle JDK 1.7.0_67 during installation and upgrade. If you prefer to install the JDK yourself, follow the instructions in Java Development Kit Installation.
Supported Browsers
The Cloudera Manager Admin Console, which you use to install, configure, manage, and monitor services, supports the following browsers:
•	Mozilla Firefox 24 and 31.
•	Google Chrome.
•	Internet Explorer 9 and higher. Internet Explorer 11 Native Mode.
•	Safari 5 and higher.
Supported Databases
Cloudera Manager requires several databases. The Cloudera Manager Server stores information about configured services, role assignments, configuration history, commands, users, and running processes in a database of its own. You must also specify a database for the Activity Monitor and Reports Manager roles.
   Important: When processes restart, the configuration for each of the services is redeployed using information that is saved in the Cloudera Manager database. If this information is not available, your cluster will not start or function correctly. You must therefore schedule and maintain regular backups of the Cloudera Manager database in order to recover the cluster in the event of the loss of this database. 
After installing a database, upgrade to the latest patch version and apply any other appropriate updates. Available updates may be specific to the operating system on which it is installed.
Cloudera Manager and its supporting services can use the following databases:
•	MariaDB 5.5
•	MySQL - 5.5 and 5.6
•	Oracle 11gR2 and 12c
•	PostgreSQL - 9.2, 9.3, and 9.4
Cloudera supports the shipped version of MariaDB, MySQL and PostgreSQL for each supported Linux distribution. 
Supported CDH and Managed Service Versions
The following versions of CDH and managed services are supported:
   Warning: Cloudera Manager 5 does not support CDH 3 and you cannot upgrade Cloudera Manager 4 to Cloudera Manager 5 if you have a cluster running CDH 3. Therefore, to upgrade CDH 3 clusters to CDH 4 using Cloudera Manager, you must use Cloudera Manager 4.
•	CDH 4 and CDH 5. The latest released versions of CDH 4 and CDH 5 are strongly recommended. For information on CDH 4 requirements. For information on CDH 5 requirements
•	Cloudera Impala - Cloudera Impala is included with CDH 5. Cloudera Impala 1.2.1 with CDH 4.1.0 or later. For more information on Cloudera Impala requirements with CDH 4.
•	Cloudera Search - Cloudera Search is included with CDH 5. Cloudera Search 1.2.0 with CDH 4.6.0. For more information on Cloudera Search requirements with CDH 4.
•	Apache Spark - 0.90 or later with CDH 4.4.0 or later.
•	Apache Accumulo - 1.4.3 with CDH 4.3.0, 1.4.4 with CDH 4.5.0, and 1.6.0 with CDH 4.6.0.
Resource Requirements
Cloudera Manager requires the following resources:
•	Disk Space
o	Cloudera Manager Server
	5 GB on the partition hosting /var.
	500 MB on the partition hosting /usr.
	For parcels, the space required depends on the number of parcels you download to the Cloudera Manager Server and distribute to Agent hosts. You can download multiple parcels of the same product, of different versions and builds. If you are managing multiple clusters, only one parcel of a product/version/build/distribution is downloaded on the Cloudera Manager Server—not one per cluster. In the local parcel repository on the Cloudera Manager Server, the approximate sizes of the various parcels are as follows:
	CDH 4.6 - 700 MB per parcel; CDH 5 (which includes Impala and Search) - 1.5 GB per parcel (packed), 2 GB per parcel (unpacked)
	Cloudera Impala - 200 MB per parcel
	Cloudera Search - 400 MB per parcel
o	Cloudera Management Service -The Host Monitor and Service Monitor databases are stored on the partition hosting /var. Ensure that you have at least 20 GB available on this partition.
o	Agents - On Agent hosts each unpacked parcel requires about three times the space of the downloaded parcel on the Cloudera Manager Server. By default unpacked parcels are located in/opt/cloudera/parcels.
•	RAM - 4 GB is recommended for most cases and is required when using Oracle databases. 2 GB may be sufficient for non-Oracle deployments with fewer than 100 hosts. However, to run the Cloudera Manager Server on a machine with 2 GB of RAM, you must tune down its maximum heap size. Otherwise the kernel may kill the Server for consuming too much RAM.
•	Python - Cloudera Manager and CDH 4 require Python 2.4 or later, but Hue in CDH 5 and package installs of CDH 5 require Python 2.6 or 2.7. All supported operating systems include Python version 2.4 or later.
•	Perl - Cloudera Manager requires perl.
Networking and Security Requirements
The hosts in a Cloudera Manager deployment must satisfy the following networking and security requirements:
•	Cluster hosts must have a working network name resolution system and correctly formatted /etc/hosts file. All cluster hosts must have properly configured forward and reverse host resolution through DNS. The /etc/hosts files must
o	Contain consistent information about hostnames and IP addresses across all hosts
o	Not contain uppercase hostnames
o	Not contain duplicate IP addresses
Also, do not use aliases, either in /etc/hosts or in configuring DNS. A properly formatted /etc/hosts file should be similar to the following example:
127.0.0.1	localhost.localdomain	localhost
192.168.1.1	cluster-01.example.com	cluster-01
192.168.1.2	cluster-02.example.com	cluster-02
192.168.1.3	cluster-03.example.com	cluster-03 
•	In most cases, the Cloudera Manager Server must have SSH access to the cluster hosts when you run the installation or upgrade wizard. You must log in using a root account or an account that has password-less sudo permission. For authentication during the installation and upgrade procedures, you must either enter the password or upload a public and private key pair for the root or sudo user account. If you want to use a public and private key pair, the public key must be installed on the cluster hosts before you use Cloudera Manager.
Cloudera Manager uses SSH only during the initial install or upgrade. Once the cluster is set up, you can disable root SSH access or change the root password. Cloudera Manager does not save SSH credentials, and all credential information is discarded when the installation is complete.
•	
•	If single user mode is not enabled, the Cloudera Manager Agent runs as root so that it can make sure the required directories are created and that processes and files are owned by the appropriate user (for example, the hdfs and mapred users).
•	No blocking is done by Security-Enhanced Linux (SELinux).
   Important: Cloudera Enterprise is supported on platforms with Security-Enhanced Linux (SELinux) enabled. However, policies need to be provided by other parties or created by the administrator of the cluster deployment. Cloudera is not responsible for policy support nor policy enforcement, nor for any issues with such. If you experience issues with SELinux, contact your OS support provider.
•	
•	IPv6 must be disabled.
•	No blocking by iptables or firewalls; port 7180 must be open because it is used to access Cloudera Manager after installation. Cloudera Manager communicates using specific ports, which must be open.
•	For RHEL and CentOS, the /etc/sysconfig/network file on each host must contain the hostname you have just set (or verified) for that host.
•	Cloudera Manager and CDH use several user accounts and groups to complete their tasks. The set of user accounts and groups varies according to the components you choose to install. Do not delete these accounts or groups and do not modify their permissions and rights. Ensure that no existing systems prevent these accounts and groups from functioning. For example, if you have scripts that delete user accounts not in a whitelist, add these accounts to the list of permitted accounts. Cloudera Manager, CDH, and managed services create and use the following accounts and groups:











Installing Cloudera Manager, CDH, and Managed Services
The following diagram illustrates the phases required to install Cloudera Manager and a Cloudera Manager deployment of CDH and managed services. Every phase is required, but you can accomplish each phase in multiple ways, depending on your organization's policies and requirements.
 
The six phases are grouped into three installation paths based on how the Cloudera Manager Server and database software are installed on the Cloudera Manager Server and cluster hosts. 





Installation Manual Installation Using Cloudera Manager Packages
The steps in this topic first install Cloudera Manager and then you manually install the JDK, Cloudera Manager agents, CDH software, managed service software, configure and start your cluster.
You can also use Cloudera manager to install the JDK, CDH, the agents and other software.
To install the Cloudera Manager Server using packages, follow the instructions in this section. You can also use Puppet or Chef to install the packages. The general steps in the procedure for Installation Path B follow.
1.	Before You Begin
1.	Perform Configuration Required by Single User Mode
2.	(CDH 5 only) On RHEL 5 and CentOS 5, Install Python 2.6 or 2.7
3.	Install and Configure Databases
2.	Establish Your Cloudera Manager Repository Strategy
1.	RHEL-compatible
2.	SLES
3.	Ubuntu or Debian
1.	Install the Oracle JDK
2.	Install the Cloudera Manager Server Packages
3.	Set up a Database for the Cloudera Manager Server
4.	Manually Install the Oracle JDK, Cloudera Manager Agent, and CDH and Managed Service Packages
1.	Install the Oracle JDK
2.	Install Cloudera Manager Agent Packages
3.	Install CDH and Managed Service Packages
4.	(Optional) Install Key Trustee KMS
1.	Start the Cloudera Manager Server
2.	Start the Cloudera Manager Agents
3.	Start and Log into the Cloudera Manager Admin Console
4.	Choose Cloudera Manager Edition and Hosts
5.	Choose the Software Installation Type and Install Software
6.	Add Services
7.	Change the Default Administrator Password
8.	Configure Oozie Data Purge Settings
9.	Test the Installation
During Cloudera Manager installation you can choose to install CDH and managed service as parcels or packages. For packages, you can choose to have Cloudera Manager install the packages or install them yourself.
Before You Begin
(CDH 5 only) On RHEL 5 and CentOS 5, Install Python 2.6 or 2.7
CDH 5 Hue will only work with the default system Python version of the operating system it is being installed on. For example, on RHEL/CentOS 6 you will need Python 2.6 to start Hue.
To install packages from the EPEL repository, download the appropriate repository rpm packages to your machine and then install Python using yum. For example, use the following commands for RHEL 5 or CentOS 5:
$ su -c 'rpm -Uvh http://download.fedoraproject.org/pub/epel/5/i386/epel-release-5-4.noarch.rpm'
...
$ yum install python26
Establish Your Cloudera Manager Repository Strategy
Cloudera recommends installing products using package management tools such as yum for RHEL compatible systems, zypper for SLES, and apt-get for Debian/Ubuntu. These tools depend on access to repositories to install software. For example, Cloudera maintains Internet-accessible repositories for CDH and Cloudera Manager installation files. Strategies for installing Cloudera Manager include:
•	Standard Cloudera repositories. For this method, ensure you have added the required repository information to your systems. 
•	Internally hosted repositories. You might use internal repositories for environments where hosts do not have access to the Internet. When using an internal repository, you must copy the repo or list file to the Cloudera Manager Server host and update the repository properties to point to internal repository URLs.
RHEL-compatible
1.	Save the appropriate Cloudera Manager repo file (cloudera-manager.repo) for your system:
OS Version	Repo URL
RHEL/CentOS/Oracle 5	h tps://archive.cloudera.a-manager.repo

RHEL/CentOS 6	https://archive.cloudera.com/cm5/redhat/6/x86_64/cm/cloudera-manager.repo

RHEL/CentOS 7	https://archive.cloudera.com/cm5/redhat/7/x86_64/cm/cloudera-manager.repo

2.	
3.	Copy the repo file to the /etc/yum.repos.d/ directory.
SLES
1.	Run the following command:
$ sudo zypper addrepo -f https://archive.cloudera.com/cm5/sles/11/x86_64/cm/cloudera-manager.repo
2.	
3.	Update your system package index by running:
$ sudo zypper refresh
4.	
Ubuntu or Debian
1.	Save the appropriate Cloudera Manager list file (cloudera.list) for your system:
OS Version	Repo URL
    Ubuntu Trusty (14.04)	    https://archive.cloudera.com/cm5/ubuntu/trusty/amd64/cm/cloudera.list

    Ubuntu Precise (12.04)	    https://archive.cloudera.com/cm5/ubuntu/precise/amd64/cm/cloudera.list

    Ubuntu Lucid (10.04)	    https://archive.cloudera.com/cm5/ubuntu/lucid/amd64/cm/cloudera.list

    Debian Wheezy (7.0 and 7.1)	    https://archive.cloudera.com/cm5/debian/wheezy/amd64/cm/cloudera.list

    Debian Wheezy (6.0)	    https://archive.cloudera.com/cm5/debian/squeeze/amd64/cm/cloudera.list

2.	
3.	Copy the content of that file to the cloudera-manager.list file in the /etc/apt/sources.list.d/ directory.
4.	Update your system package index by running:
$ sudo apt-get update
5.	
Install the Oracle JDK
Install the Oracle Java Development Kit (JDK) on the Cloudera Manager Server host.
The JDK is included in the Cloudera Manager 5 repositories. After downloading and editing the repo or list file, install the JDK as follows:
  OS	Command
    RHEL	$ sudo yum install oracle-j2sdk1.7
    SLES	$ sudo zypper install oracle-j2sdk1.7
    Ubuntu or Debian	$ sudo apt-get install oracle-j2sdk1.7
Install the Cloudera Manager Server Packages
1.	Install the Cloudera Manager Server packages either on the host where the database is installed, or on a host that has access to the database. This host need not be a host in the cluster that you want to manage with Cloudera Manager. On the Cloudera Manager Server host, type the following commands to install the Cloudera Manager packages.
 OS	Command
   RHEL, if you have a yum repo configured	$ sudo yum install cloudera-manager-daemons cloudera-manager-server
   RHEL,if you're manually transferring RPMs	$ sudo yum --nogpgcheck localinstall cloudera-manager-daemons-*.rpm
$ sudo yum --nogpgcheck localinstall cloudera-manager-server-*.rpm
   SLES	$ sudo zypper install cloudera-manager-daemons cloudera-manager-server 
   Ubuntu or Debian	$ sudo apt-get install cloudera-manager-daemons cloudera-manager-server 
2.	
3.	If you choose an Oracle database for use with Cloudera Manager, edit the /etc/default/cloudera-scm-server file on the Cloudera Manager server host. Locate the line that begins with export CM_JAVA_OPTS and change the -Xmx2G option to -Xmx4G.
Set up a Database for the Cloudera Manager Server
Depending on whether you are using an external database, or the embedded PostgreSQL database, do one of the following:
•	External database - Prepare the Cloudera Manager Server database as described in Preparing a Cloudera Manager Server External Database.
•	Embedded database - Install an embedded PostgreSQL database as described in Installing and Starting the Cloudera Manager Server Embedded Database.
Manually Install the Oracle JDK, Cloudera Manager Agent, and CDH and Managed Service Packages
You can use Cloudera Manager to install the Oracle JDK, Cloudera Manager Agent packages, CDH, and managed service packages in Choose the Software Installation Type and Install Software, or you can install them manually. To use Cloudera Manager to install the packages, you must meet the requirements described in Cloudera Manager Deployment.
   Important: If you are installing CDH and managed service software using packages and you want to manually install Cloudera Manager Agent or CDH packages, you must manually install them both following the procedures in this section; you cannot choose to install only one of them this way.
If you are going to use Cloudera Manager to install software, skip this section and go to Start the Cloudera Manager Server. Otherwise, to manually install software, proceed with the steps in this section.
Install the Oracle JDK
Install the Oracle JDK on the cluster hosts. Cloudera Manager 5 can manage both CDH 5 and CDH 4, and the required JDK version varies accordingly:
•	CDH 5 - Java Development Kit Installation.
•	CDH 4 - Java Development Kit Installation.
Install Cloudera Manager Agent Packages
To install the Cloudera Manager Agent packages manually, do the following on every Cloudera Manager Agent host (including those that will run one or more of the Cloudera Management Service roles: Service Monitor, Activity Monitor, Event Server, Alert Publisher, or Reports Manager):
1.	Use one of the following commands to install the Cloudera Manager Agent packages:
 OS	Command
   RHEL, if you have a yum repo configured:	$ sudo yum install cloudera-manager-agent cloudera-manager-daemons
   RHEL, if you're manually transferring RPMs:	$ sudo yum localinstall cloudera-manager-agent-package.*.x86_64.rpm cloudera-manager-daemons
   SLES	$ sudo zypper install cloudera-manager-agent cloudera-manager-daemons
   Ubuntu or Debian	$ sudo apt-get install cloudera-manager-agent cloudera-manager-daemons
2.	
3.	On every Cloudera Manager Agent host, configure the Cloudera Manager Agent to point to the Cloudera Manager Server by setting the following properties in the /etc/cloudera-scm-agent/config.ini configuration file:
  Property	Description
    server_host	    Name of the host where Cloudera Manager Server is running.
    server_port	    Port on the host where Cloudera Manager Server is running.
Install CDH and Managed Service Packages
1.	Choose a repository strategy:
o	Standard Cloudera repositories. For this method, ensure you have added the required repository information to your systems.
o	Internally hosted repositories. You might use internal repositories for environments where hosts do not have access to the Internet. For information about preparing your environment, see Understanding Custom Installation Solutions. When using an internal repository, you must copy the repo or list file to the Cloudera Manager Server host and update the repository properties to point to internal repository URLs.
2.	Install packages:
CDH Version	Procedure
   CDH 5	o	Red Hat
1.	Download and install the "1-click Install" package.
1.	Download the CDH 5 "1-click Install" package (or RPM).
Click the appropriate RPM and Save File to a directory with write access (for example, your home directory).
2.	
OS Version	Link to CDH 5 RPM
      RHEL /CentOS/Oracle 5	    RHEL/CentOS/Oracle 5 link

      RHEL/CentOS/Oracle 6	    RHEL/CentOS/Oracle 6 link

      RHEL/CentOS/Oracle 7	    RHEL/CentOS/Oracle 7 link

3.	
4.	Install the RPM for all RHEL versions:
$ sudo yum --nogpgcheck localinstall cloudera-cdh-5-0.x86_64.rpm 
5.	
2.	(Optionally) add a repository key:
	Red Hat/CentOS/Oracle 5
$ sudo rpm --import https://archive.cloudera.com/cdh5/redhat/5/x86_64/cdh/RPM-GPG-KEY-cloudera
	
	Red Hat/CentOS/Oracle 6
$ sudo rpm --import https://archive.cloudera.com/cdh5/redhat/6/x86_64/cdh/RPM-GPG-KEY-cloudera
	
3.	Install the CDH packages:
$ sudo yum clean all
$ sudo yum install avro-tools crunch flume-ng hadoop-hdfs-fuse hadoop-hdfs-nfs3 hadoop-httpfs hadoop-kms hbase-solr hive-hbase hive-webhcat hue-beeswax hue-hbase hue-impala hue-pig hue-plugins hue-rdbms hue-search hue-spark hue-sqoop hue-zookeeper impala impala-shell kite llama mahout oozie pig pig-udf-datafu search sentry solr-mapreduce spark-core spark-master spark-worker spark-history-server spark-python sqoop sqoop2 whirr
4.	
   Note: Installing these packages also installs all the other CDH packages required for a full CDH 5 installation.
5.	
     SLES
1.	Download and install the "1-click Install" package.
1.	Download the CDH 5 "1-click Install" package.
Download the rpm file, choose Save File, and save it to a directory to which you have write access (for example, your home directory).
2.	
     Install the RPM:
$ sudo rpm -i cloudera-cdh-5-0.x86_64.rpm
o	
     Update your system package index by running:
$ sudo zypper refresh
3.	
     (Optionally) add a repository key:
$ sudo rpm --import https://archive.cloudera.com/cdh5/sles/11/x86_64/cdh/RPM-GPG-KEY-cloudera
1.	
     Install the CDH packages:
$ sudo zypper clean --all
$ sudo zypper install avro-tools crunch flume-ng hadoop-hdfs-fuse hadoop-hdfs-nfs3 hadoop-httpfs hadoop-kms 
hbase-solr hive-hbase hive-webhcat hue-beeswax hue-hbase hue-impala hue-pig hue-plugins hue-rdbms hue-search 
hue-spark hue-sqoop hue-zookeeper impala impala-shell kite llama mahout oozie pig pig-udf-datafu search sentry 
solr-mapreduce spark-core spark-master spark-worker spark-history-server spark-python sqoop sqoop2 whirr
2.	
   Note: Installing these packages also installs all the other CDH packages required for 
a full CDH 5 installation.
3.	
    Ubuntu and Debian
    Download and install the "1-click Install" package
    Download the CDH 5 "1-click Install" package:
OS Version	Package Link
      Wheezy	    Wheezy package

      Precise  	    Precise package

      Trusty	    Trusty package

1.	
     Install the package by doing one of the following:
     Choose Open with in the download window to use the package manager.
      Choose Save File, save the package to a directory to which you have write access (for example, 
      your home directory), and install it from the command line. For example:
sudo dpkg -i cdh5-repository_1.0_all.deb
	
1.	Optionally add a repository key:
	Debian Wheezy
$ curl -s https://archive.cloudera.com/cdh5/debian/wheezy/amd64/cdh/archive.key | sudo apt-key add -
	
	Ubuntu Precise
$ curl -s https://archive.cloudera.com/cdh5/ubuntu/precise/amd64/cdh/archive.key | sudo apt-key add -
	
2.	Install the CDH packages:
$ sudo apt-get update
$ sudo apt-get install avro-tools crunch flume-ng hadoop-hdfs-fuse hadoop-hdfs-nfs3 hadoop-httpfs hadoop-kms 
hbase-solr hive-hbase hive-webhcat hue-beeswax hue-hbase hue-impala hue-pig hue-plugins hue-rdbms hue-search 
hue-spark hue-sqoop hue-zookeeper impala impala-shell kite llama mahout oozie pig pig-udf-datafu search sentry 
solr-mapreduce spark-core spark-master spark-worker spark-history-server spark-python sqoop sqoop2 whirr
3.	
   Note: Installing these packages also installs all the other CDH packages required for a full 
CDH 5 installation.
4.	
    CDH 4, Impala, 
    and Solr	    RHEL-compatible
Click the entry in the table at CDH Download Information that matches your RHEL or CentOS system.
     Save it in the /etc/yum.repos.d/ directory.
2.	Optionally add a repository key:
	RHEL/CentOS/Oracle 5
$ sudo rpm --import https://archive.cloudera.com/cdh4/redhat/5/x86_64/cdh/RPM-GPG-KEY-cloudera
	
	RHEL/CentOS 6
$ sudo rpm --import https://archive.cloudera.com/cdh4/redhat/6/x86_64/cdh/RPM-GPG-KEY-cloudera
	
3.	Install packages on every host in your cluster:
1.	Install CDH 4 packages:
$ sudo yum -y install bigtop-utils bigtop-jsvc bigtop-tomcat hadoop hadoop-hdfs hadoop-httpfs 
hadoop-mapreduce hadoop-yarn hadoop-client hadoop-0.20-mapreduce hue-plugins hbase hive oozie 
oozie-client pig zookeeper
2.	
To install the hue-common package and all Hue applications on the Hue host, install the 
hue meta-package:
$ sudo yum install hue 
3.	
    (Requires CDH 4.2 or later) Install Impala
     In the table at Cloudera Impala Version and Download Information, click the entry that matches your 
     RHEL or CentOS system.
     Navigate to the repo file for your system and save it in the /etc/yum.repos.d/ directory.
     Install Impala and the Impala Shell on Impala machines:
$ sudo yum -y install impala impala-shell
1.	
    (Requires CDH 4.3 or later) Install Search
In the table at Cloudera Search Version and Download Information, click the entry that matches your 
RHEL or CentOS system.
    Navigate to the repo file for your system and save it in the /etc/yum.repos.d/ directory.
    Install the Solr Server on machines where you want Cloudera Search.
$ sudo yum -y install solr-server
1.	
    SLES
    Run the following command:
$ sudo zypper addrepo -f https://archive.cloudera.com/cdh4/sles/11/x86_64/cdh/cloudera-cdh4.repo
1.	
    Update your system package index by running:
$ sudo zypper refresh
2.	
     Optionally add a repository key:
$ sudo rpm --import https://archive.cloudera.com/cdh4/sles/11/x86_64/cdh/RPM-GPG-KEY-cloudera  
3.	
     Install packages on every host in your cluster:
     Install CDH 4 packages:
$ sudo zypper install bigtop-utils bigtop-jsvc bigtop-tomcat hadoop hadoop-hdfs hadoop-httpfs 
hadoop-mapreduce hadoop-yarn hadoop-client hadoop-0.20-mapreduce hue-plugins hbase hive oozie 
oozie-client pig zookeeper
1.	
     To install the hue-common package and all Hue applications on the Hue host, install the hue 
     meta-package:
$ sudo zypper install hue 
2.	
     (Requires CDH 4.2 or later) Install Impala
     Run the following command:
$ sudo zypper addrepo -f https://archive.cloudera.com/impala/sles/11/x86_64/impala/cloudera-impala.repo
1.	
     Install Impala and the Impala Shell on Impala machines:
$ sudo zypper install impala impala-shell
2.	
    (Requires CDH 4.3 or later) Install Search
     Run the following command:
$ sudo zypper addrepo -f https://archive.cloudera.com/search/sles/11/x86_64/search/cloudera-search.repo
1.	
    Install the Solr Server on machines where you want Cloudera Search.
$ sudo zypper install solr-server
2.	
     Ubuntu or Debian
     In the table at CDH Version and Packaging Information, click the entry that matches your Ubuntu 
      or Debian system.
     Navigate to the list file (cloudera.list) for your system and save it in the /etc/apt/sources.list.d/ 
     directory. For example, to install CDH 4 for 64-bit Ubuntu Lucid, your cloudera.list file should look 
     like:
deb [arch=amd64] https://archive.cloudera.com/cdh4/ubuntu/lucid/amd64/cdh lucid-cdh4 contrib
deb-src https://archive.cloudera.com/cdh4/ubuntu/lucid/amd64/cdh lucid-cdh4 contrib
1.	
     Optionally add a repository key:
     Ubuntu Lucid
$ curl -s https://archive.cloudera.com/cdh4/ubuntu/lucid/amd64/cdh/archive.key | sudo apt-key add -
	
    Ubuntu Precise
$ curl -s https://archive.cloudera.com/cdh4/ubuntu/precise/amd64/cdh/archive.key | sudo apt-key add -
	
	Debian Squeeze
$ curl -s https://archive.cloudera.com/cdh4/debian/squeeze/amd64/cdh/archive.key | sudo apt-key add -
	
    Install packages on every host in your cluster:
    Install CDH 4 packages:
$ sudo apt-get install bigtop-utils bigtop-jsvc bigtop-tomcat hadoop hadoop-hdfs hadoop-httpfs hadoop-mapreduce hadoop-yarn hadoop-client hadoop-0.20-mapreduce hue-plugins hbase hive oozie oozie-client pig zookeeper
1.	
     To install the hue-common package and all Hue applications on the Hue host, install the hue 
     meta-package:
$ sudo apt-get install hue 
2.	
     (Requires CDH 4.2 or later) Install Impala
     In the table at Cloudera Impala Version and Download Information, click the entry that matches your 
     Ubuntu or Debian system.
     Navigate to the list file for your system and save it in the /etc/apt/sources.list.d/ directory.
     Install Impala and the Impala Shell on Impala machines:
$ sudo apt-get install impala impala-shell
1.	
    (Requires CDH 4.3 or later) Install Search
In the table at Cloudera Search Version and Download Information, click the entry that matches your 
Ubuntu or Debian system.
    Install Solr Server on machines where you want Cloudera Search:
$ sudo apt-get install solr-server
1.	
1.	
(Optional) Install Key Trustee KMS
If you want to use Cloudera Navigator Key Trustee Server as the underlying key store for HDFS Data At Rest Encryption, install the Key Trustee KMS.
   Important: Following these instructions will install the required software to add the Key Trustee KMS service to your cluster; this enables you to use Cloudera Navigator Key Trustee Server as the underlying keystore for HDFS Data At Rest Encryption. This does not install Key Trustee Server. 
To install the Key Trustee KMS:
1.	You must create an internal repository to install the Cloudera Navigator data encryption components. For instructions on creating an internal repository, see Creating and Using a Package Repository for Cloudera Manager.
2.	Add the internal repository you created. See Modifying Clients to Find the Repository for more information.
3.	Install the keytrustee-keyprovider package, using the appropriate command for your operating system:
o	RHEL-compatible
$ sudo yum install keytrustee-keyprovider
o	
o	SLES
$ sudo zypper install keytrustee-keyprovider
o	
o	Ubuntu or Debian
$ sudo apt-get install keytrustee-keyprovider
o	
Start the Cloudera Manager Server
   Important: When you start the Cloudera Manager Server and Agents, Cloudera Manager assumes you are not already running HDFS and MapReduce. If these services are running:
1.	Shut down HDFS and MapReduce. See Stopping Services (CDH 4) or Stopping CDH Services Using the Command Line (CDH 5) for the commands to stop these services.
2.	Configure the init scripts to not start on boot. Use commands similar to those shown in Configuring init to Start Core Hadoop System Services(CDH 4) or Configuring init to Start Hadoop System Services (CDH 5), but disable the start on boot (for example, $ sudo chkconfig hadoop-hdfs-namenode off).
Contact Cloudera Support for help converting your existing Hadoop configurations for use with Cloudera Manager.
1.	Run this command on the Cloudera Manager Server host:
$ sudo service cloudera-scm-server start
2.	If the Cloudera Manager Server does not start, see Troubleshooting Installation and Upgrade Problems.
Start the Cloudera Manager Agents
If you are going to use Cloudera Manager to install Cloudera Manager Agent packages, skip this section and go to Start and Log into the Cloudera Manager Admin Console. Otherwise, run this command on each Agent host:
$ sudo service cloudera-scm-agent start
When the Agent starts, it contacts the Cloudera Manager Server. If communication fails between a Cloudera Manager Agent and Cloudera Manager Server, see Troubleshooting Installation and Upgrade Problems.
When the Agent hosts reboot, cloudera-scm-agent starts automatically.
Start and Log into the Cloudera Manager Admin Console
The Cloudera Manager Server URL takes the following form http://Server host:port, where Server host is the fully qualified domain name or IP address of the host where the Cloudera Manager Server is installed, and port is the port configured for the Cloudera Manager Server. The default port is 7180.
1.	Wait several minutes for the Cloudera Manager Server to complete its startup. To observe the startup process, run 
tail -f /var/log/cloudera-scm-server/cloudera-scm-server.log 
on the Cloudera Manager Server host.
2.	In a web browser, enter http://Server host:7180, where Server host is the fully qualified domain name or IP address of the host where the Cloudera Manager Server is running. The login screen for Cloudera Manager Admin Console displays.
3.	Log into Cloudera Manager Admin Console. The default credentials are: Username: admin Password: admin. Cloudera Manager does not support changing the admin username for the installed account. You can change the password using Cloudera Manager after you run the installation wizard. Although you cannot change the admin username, you can add a new user, assign administrative privileges to the new user, and then delete the default admin account.
4.	After logging in, the Cloudera Manager End User License Terms and Conditions page displays. Read the terms and conditions and then select Yes to accept them.
5.	Click Continue.
Choose Cloudera Manager Edition and Hosts
Choose which edition of Cloudera Manager you are using and which hosts will run CDH and managed services.
1.	When you start the Cloudera Manager Admin Console, the install wizard starts up. Click Continue to get started.
2.	Choose which edition to install:
o	Cloudera Express, which does not require a license, but provides a limited set of features.
o	Cloudera Enterprise Data Hub Edition Trial, which does not require a license, but expires after 60 days and cannot be renewed.
o	Cloudera Enterprise with one of the following license types:
	Basic Edition
	Flex Edition
	Data Hub Edition
If you choose Cloudera Express or Cloudera Enterprise Data Hub Edition Trial, you can upgrade the license at a later time. See Managing Licenses.
3.	If you elect Cloudera Enterprise, install a license:
1.	Click Upload License.
2.	Click the document icon to the left of the Select a License File text field.
3.	Navigate to the location of your license file, click the file, and click Open.
4.	Click Upload.
Click Continue to proceed with the installation.
1.	Information is displayed indicating what the CDH installation includes. At this point, you can access online Help or the Support Portal. Click Continueto proceed with the installation.
2.	Do one of the following depending on whether you are using Cloudera Manager to install software:
o	If you are using Cloudera Manager to install software, search for and choose hosts:
1.	To enable Cloudera Manager to automatically discover hosts on which to install CDH and managed services, enter the cluster hostnames or IP addresses. You can also specify hostname and IP address ranges. For example:
 Range Definition	Matching Hosts
    10.1.1.[1-4]	10.1.1.1, 10.1.1.2, 10.1.1.3, 10.1.1.4
    host[1-3].company.com	   host1.company.com, host2.company.com, host3.company.com
    host[07-10].company.com	   host07.company.com, host08.company.com, host09.company.com, 
   host10.company.com
2.	
You can specify multiple addresses and address ranges by separating them by commas, semicolons, tabs, or blank spaces, or by placing them on separate lines. Use this technique to make more specific searches instead of searching overly wide ranges. The scan results will include all addresses scanned, but only scans that reach hosts running SSH will be selected for inclusion in your cluster by default. If you don't know the IP addresses of all of the hosts, you can enter an address range that spans over unused addresses and then deselect the hosts that do not exist (and are not discovered) later in this procedure. However, keep in mind that wider ranges will require more time to scan.
3.	
4.	Click Search. Cloudera Manager identifies the hosts on your cluster to allow you to configure them for services. If there are a large number of hosts on your cluster, wait a few moments to allow them to be discovered and shown in the wizard. If the search is taking too long, you can stop the scan by clicking Abort Scan. To find additional hosts, click New Search, add the host names or IP addresses and click Search again. Cloudera Manager scans hosts by checking for network connectivity. If there are some hosts where you want to install services that are not shown in the list, make sure you have network connectivity between the Cloudera Manager Server host and those hosts. Common causes of loss of connectivity are firewalls and interference from SELinux.
5.	Verify that the number of hosts shown matches the number of hosts where you want to install services. Deselect host entries that do not exist and deselect the hosts where you do not want to install services. Click Continue. The Select Repository screen displays.
o	If you installed Cloudera Agent packages in Install Cloudera Manager Agent Packages, choose from among hosts with the packages installed:
1.	Click the Currently Managed Hosts tab.
2.	Choose the hosts to add to the cluster.
1.	Click Continue.
Choose the Software Installation Type and Install Software
Choose a software installation type (parcels or packages) and install the software if not previously installed.
1.	Choose the software installation type and CDH and managed service version:
o	Use Parcels
1.	Choose the parcels to install. The choices depend on the repositories you have chosen; a repository can contain multiple parcels. Only the parcels for the latest supported service versions are configured by default.
You can add additional parcels for previous versions by specifying custom repositories. For example, you can find the locations of the previous CDH 4 parcels at https://archive.cloudera.com/cdh4/parcels/. Or, if you are installing CDH 4.3 and want to use policy-file authorization, you can add the Sentry parcel using this mechanism.
2.	
1.	To specify the parcel directory, specify the local parcel repository, add a parcel repository, or specify the properties of a proxy server through which parcels are downloaded, click the More Options button and do one or more of the following:
	Parcel Directory and Local Parcel Repository Path - Specify the location of parcels on cluster hosts and the Cloudera Manager Server host. If you change the default value for Parcel Directory and have already installed and started Cloudera Manager Agents, restart the Agents:
$ sudo service cloudera-scm-agent restart
	
	Parcel Repository - In the Remote Parcel Repository URLs field, click the   button and enter the URL of the repository. The URL you specify is added to the list of repositories listed in the Configuring Cloudera Manager Server Parcel Settings page and a parcel is added to the list of parcels on the Select Repository page. If you have multiple repositories configured, you see all the unique parcels contained in all your repositories.
	Proxy Server - Specify the properties of a proxy server.
2.	Click OK.
1.	Select the release of Cloudera Manager Agent. You can choose either the version that matches the Cloudera Manager Server you are currently using or specify a version in a custom repository. If you opted to use custom repositories for installation files, you can provide a GPG key URL that applies for all repositories. Click Continue.
o	Use Packages - Do one of the following:
	If Cloudera Manager is installing the packages:
1.	Click the package version.
2.	Select the release of Cloudera Manager Agent. You can choose either the version that matches the Cloudera Manager Server you are currently using or specify a version in a custom repository. If you opted to use custom repositories for installation files, you can provide a GPG key URL that applies for all repositories. Click Continue.
	If you manually installed packages in Install CDH and Managed Service Packages, select the CDH version (CDH 4 or CDH 5) that matches the packages you installed manually.
1.	Select the Install Oracle Java SE Development Kit (JDK) checkbox to allow Cloudera Manager to install the JDK on each cluster host or leave deselected if you installed it. If checked, your local laws permit you to deploy unlimited strength encryption, and you are running a secure cluster, select the Install Java Unlimited Strength Encryption Policy Files checkbox. Click Continue.
2.	(Optional) Select Single User Mode to configure the Cloudera Manager Agent and all service processes to run as the same user. This mode requiresextra configuration steps that must be done manually on all hosts in the cluster. If you have not performed the steps, directory creation will fail in the installation wizard. In most cases, you can create the directories but the steps performed by the installation wizard may have to be continued manually. Click Continue.
3.	If you chose to have Cloudera Manager install software, specify host installation properties:
o	Select root or enter the user name for an account that has password-less sudo permission.
o	Select an authentication method:
	If you choose password authentication, enter and confirm the password.
	If you choose public-key authentication, provide a passphrase and path to the required key files.
o	You can specify an alternate SSH port. The default value is 22.
o	You can specify the maximum number of host installations to run at once. The default value is 10.
4.	Click Continue. If you chose to have Cloudera Manager install software, Cloudera Manager installs the Oracle JDK, Cloudera Manager Agent, packages and CDH and managed service parcels or packages. During parcel installation, progress is indicated for the phases of the parcel installation process in separate progress bars. If you are installing multiple parcels, you see progress bars for each parcel. When the Continue button at the bottom of the screen turns blue, the installation process is completed.
5.	Click Continue. The Host Inspector runs to validate the installation and provides a summary of what it finds, including all the versions of the installed components. If the validation is successful, click Finish.
Add Services
Use the Cloudera Manager wizard to configure and start CDH and managed services.
1.	In the first page of the Add Services wizard, choose the combination of services to install and whether to install Cloudera Navigator:
o	Click the radio button next to the combination of services to install:
CDH 4	CDH 5
     Core Hadoop - HDFS, MapReduce, ZooKeeper, Oozie, Hive, and Hue
     Core with HBase
     Core with Impala
     All Services - HDFS, MapReduce, ZooKeeper, HBase, Impala, Oozie, Hive, Hue, and Sqoop
     Custom Services - Any combination of services.	     Core Hadoop - HDFS, YARN (includes MapReduce 2), 
     ZooKeeper, Oozie, Hive, and Hue
      Core with HBase
      Core with Impala
      Core with Search
\     Core with Spark
      All Services - HDFS, YARN (includes MapReduce 2),
      ZooKeeper, Oozie, Hive, Hue, HBase, Impala, Solr, 
      Spark, and Key-Value Store Indexer
      Custom Services - Any combination of services.
o	As you select services, keep the following in mind:
	Some services depend on other services; for example, HBase requires HDFS and ZooKeeper. Cloudera Manager tracks dependencies and installs the correct combination of services.
	In a Cloudera Manager deployment of a CDH 4 cluster, the MapReduce service is the default MapReduce computation framework.Choose Custom Services to install YARN, or use the Add Service functionality to add YARN after installation completes.
   Note: You can create a YARN service in a CDH 4 cluster, but it is not considered production ready.
	
	In a Cloudera Manager deployment of a CDH 5 cluster, the YARN service is the default MapReduce computation framework. ChooseCustom Services to install MapReduce, or use the Add Service functionality to add MapReduce after installation completes.
   Note: In CDH 5, the MapReduce service has been deprecated. However, the MapReduce service is fully supported for backward compatibility through the CDH 5 lifecycle.
	
	The Flume service can be added only after your cluster has been set up.
o	If you have chosen Data Hub Edition Trial or Cloudera Enterprise, optionally select the Include Cloudera Navigator checkbox to enable Cloudera Navigator. See the Cloudera Navigator Documentation.
Click Continue.
2.	Customize the assignment of role instances to hosts. The wizard evaluates the hardware configurations of the hosts to determine the best hosts for each role. The wizard assigns all worker roles to the same set of hosts to which the HDFS DataNode role is assigned. You can reassign role instances if necessary.
Click a field below a role to display a dialog containing a list of hosts. If you click a field containing multiple hosts, you can also select All Hosts to assign the role to all hosts, or Custom to display the pageable hosts dialog.
3.	
The following shortcuts for specifying hostname patterns are supported:
4.	
o	Range of hostnames (without the domain portion)
 Range Definition	Matching Hosts
    10.1.1.[1-4]	    10.1.1.1, 10.1.1.2, 10.1.1.3, 10.1.1.4
    host[1-3].company.com	    host1.company.com, host2.company.com, host3.company.com
    host[07-10].company.com	    host07.company.com, host08.company.com, host09.company.com, host10.company.com
o	
o	IP addresses
o	Rack name
Click the View By Host button for an overview of the role assignment by hostname ranges.
5.	When you are satisfied with the assignments, click Continue.
6.	On the Database Setup page, configure settings for required databases:
1.	Enter the database host, database type, database name, username, and password for the database that you created when you set up the database.
2.	Click Test Connection to confirm that Cloudera Manager can communicate with the database using the information you have supplied. If the test succeeds in all cases, click Continue; otherwise, check and correct the information you have provided for the database and then try the test again. (For some servers, if you are using the embedded database, you will see a message saying the database will be created at a later step in the installation process.) The Review Changes screen displays.
1.	Review the configuration changes to be applied. Confirm the settings entered for file system paths. The file paths required vary based on the services to be installed. If you chose to add the Sqoop service, indicate whether to use the default Derby database or the embedded PostgreSQL database. If the latter, type the database name, host, and user credentials that you specified when you created the database.
   Warning: Do not place DataNode data directories on NAS devices. When resizing an NAS, block replicas can be deleted, which will result in reports of missing blocks.
2.	Click Continue. The wizard starts the services.
3.	When all of the services are started, click Continue. You see a success message indicating that your cluster has been successfully started.
4.	Click Finish to proceed to the Cloudera Manager Admin Console Home Page.
Change the Default Administrator Password
As soon as possible, change the default administrator password:
1.	Right-click the logged-in username at the far right of the top navigation bar and select Change Password.
2.	Enter the current password and a new password twice, and then click Update.
Configure Oozie Data Purge Settings
If you added an Oozie service, you can change your Oozie configuration to control when data is purged in order to improve performance, cut down on database disk usage, or to keep the history for a longer period of time. Limiting the size of the Oozie database can also improve performance during upgrades. 

