//Deprecated: in favor of universe.dagger.io/alpha package
package ansible

#DefaultAnsibleConfig: #"""
	    # Example config file for ansible -- https://ansible.com/\n
	    # =======================================================\n
	    \n
	    # Nearly all parameters can be overridden in ansible-playbook\n
	    # or with command line flags. Ansible will read ANSIBLE_CONFIG,\n
	    # ansible.cfg in the current working directory, .ansible.cfg in\n
	    # the home directory, or /etc/ansible/ansible.cfg, whichever it\n
	    # finds first\n
	    \n
	    # For a full list of available options, run ansible-config list or see the\n
	    # documentation: https://docs.ansible.com/ansible/latest/reference_appendices/config.html.\n
	    \n
	    [defaults]\n
	    inventory       = /etc/ansible/hosts\n
	    library         = ~/.ansible/plugins/modules:/usr/share/ansible/plugins/modules\n
	    module_utils    = ~/.ansible/plugins/module_utils:/usr/share/ansible/plugins/module_utils\n
	    remote_tmp      = ~/.ansible/tmp\n
	    local_tmp       = ~/.ansible/tmp\n
	    forks           = 5\n
	    poll_interval   = 15\n
	    ask_pass        = False\n
	    transport       = smart\n
	    \n
	    # Plays will gather facts by default, which contain information about\n
	    # the remote system.\n
	    #\n
	    # smart - gather by default, but don't regather if already gathered\n
	    # implicit - gather by default, turn off with gather_facts: False\n
	    # explicit - do not gather by default, must say gather_facts: True\n
	    #gathering = implicit\n
	    \n
	    # This only affects the gathering done by a play's gather_facts directive,\n
	    # by default gathering retrieves all facts subsets\n
	    # all - gather all subsets\n
	    # network - gather min and network facts\n
	    # hardware - gather hardware facts (longest facts to retrieve)\n
	    # virtual - gather min and virtual facts\n
	    # facter - import facts from facter\n
	    # ohai - import facts from ohai\n
	    # You can combine them using comma (ex: network,virtual)\n
	    # You can negate them using ! (ex: !hardware,!facter,!ohai)\n
	    # A minimal set of facts is always gathered.\n
	    #\n
	    #gather_subset = all\n
	    \n
	    # some hardware related facts are collected\n
	    # with a maximum timeout of 10 seconds. This\n
	    # option lets you increase or decrease that\n
	    # timeout to something more suitable for the\n
	    # environment.\n
	    #\n
	    #gather_timeout = 10\n
	    \n
	    # Ansible facts are available inside the ansible_facts.* dictionary\n
	    # namespace. This setting maintains the behaviour which was the default prior\n
	    # to 2.5, duplicating these variables into the main namespace, each with a\n
	    # prefix of 'ansible_'.\n
	    # This variable is set to True by default for backwards compatibility. It\n
	    # will be changed to a default of 'False' in a future release.\n
	    #\n
	    #inject_facts_as_vars = True\n
	    \n
	    # Paths to search for collections, colon separated\n
	    # collections_paths = ~/.ansible/collections:/usr/share/ansible/collections\n
	    \n
	    # Paths to search for roles, colon separated\n
	    #roles_path = ~/.ansible/roles:/usr/share/ansible/roles:/etc/ansible/roles\n
	    \n
	    # Host key checking is enabled by default\n
	    #host_key_checking = True\n
	    \n
	    # You can only have one 'stdout' callback type enabled at a time. The default\n
	    # is 'default'. The 'yaml' or 'debug' stdout callback plugins are easier to read.\n
	    #\n
	    #stdout_callback = default\n
	    #stdout_callback = yaml\n
	    #stdout_callback = debug\n
	    \n
	    \n
	    # Ansible ships with some plugins that require enabling\n
	    # this is done to avoid running all of a type by default.\n
	    # These setting lists those that you want enabled for your system.\n
	    # Custom plugins should not need this unless plugin author disables them\n
	    # by default.\n
	    #\n
	    # Enable callback plugins, they can output to stdout but cannot be 'stdout' type.\n
	    #callbacks_enabled = timer, mail\n
	    \n
	    # Determine whether includes in tasks and handlers are "static" by\n
	    # default. As of 2.0, includes are dynamic by default. Setting these\n
	    # values to True will make includes behave more like they did in the\n
	    # 1.x versions.\n
	    #\n
	    #task_includes_static = False\n
	    #handler_includes_static = False\n
	    \n
	    # Controls if a missing handler for a notification event is an error or a warning\n
	    #error_on_missing_handler = True\n
	    \n
	    # Default timeout for connection plugins\n
	    #timeout = 10\n
	    \n
	    # Default user to use for playbooks if user is not specified\n
	    # Uses the connection plugin's default, normally the user currently executing Ansible,\n
	    # unless a different user is specified here.\n
	    #\n
	    #remote_user = root\n
	    \n
	    # Logging is off by default unless this path is defined.\n
	    #log_path = /var/log/ansible.log\n
	    \n
	    # Default module to use when running ad-hoc commands\n
	    #module_name = command\n
	    \n
	    # Use this shell for commands executed under sudo.\n
	    # you may need to change this to /bin/bash in rare instances\n
	    # if sudo is constrained.\n
	    #\n
	    #executable = /bin/sh\n
	    \n
	    # By default, variables from roles will be visible in the global variable\n
	    # scope. To prevent this, set the following option to True, and only\n
	    # tasks and handlers within the role will see the variables there\n
	    #\n
	    #private_role_vars = False\n
	    \n
	    # List any Jinja2 extensions to enable here.\n
	    #jinja2_extensions = jinja2.ext.do,jinja2.ext.i18n\n
	    \n
	    # If set, always use this private key file for authentication, same as\n
	    # if passing --private-key to ansible or ansible-playbook\n
	    #\n
	    #private_key_file = /path/to/file\n
	    \n
	    # If set, configures the path to the Vault password file as an alternative to\n
	    # specifying --vault-password-file on the command line. This can also be\n
	    # an executable script that returns the vault password to stdout.\n
	    #\n
	    #vault_password_file = /path/to/vault_password_file\n
	    \n
	    # Format of string {{ ansible_managed }} available within Jinja2\n
	    # templates indicates to users editing templates files will be replaced.\n
	    # replacing {file}, {host} and {uid} and strftime codes with proper values.\n
	    #\n
	    #ansible_managed = Ansible managed: {file} modified on %Y-%m-%d %H:%M:%S by {uid} on {host}\n
	    \n
	    # {file}, {host}, {uid}, and the timestamp can all interfere with idempotence\n
	    # in some situations so the default is a static string:
	    #\n
	    #ansible_managed = Ansible managed\n
	    \n
	    # By default, ansible-playbook will display "Skipping [host]" if it determines a task\n
	    # should not be run on a host. Set this to "False" if you don't want to see these "Skipping"\n
	    # messages. NOTE: the task header will still be shown regardless of whether or not the\n
	    # task is skipped.\n
	    #\n
	    #display_skipped_hosts = True\n
	    \n
	    # By default, if a task in a playbook does not include a name: field then\n
	    # ansible-playbook will construct a header that includes the task's action but\n
	    # not the task's args. This is a security feature because ansible cannot know\n
	    # if the *module* considers an argument to be no_log at the time that the\n
	    # header is printed. If your environment doesn't have a problem securing\n
	    # stdout from ansible-playbook (or you have manually specified no_log in your\n
	    # playbook on all of the tasks where you have secret information) then you can\n
	    # safely set this to True to get more informative messages.\n
	    #\n
	    #display_args_to_stdout = False\n
	    \n
	    # Ansible will raise errors when attempting to dereference\n
	    # Jinja2 variables that are not set in templates or action lines. Uncomment this line\n
	    # to change this behavior.\n
	    #\n
	    #error_on_undefined_vars = False\n
	    \n
	    # Ansible may display warnings based on the configuration of the\n
	    # system running ansible itself. This may include warnings about 3rd party packages or\n
	    # other conditions that should be resolved if possible.\n
	    # To disable these warnings, set the following value to False:
	    #\n
	    #system_warnings = True\n
	    \n
	    # Ansible may display deprecation warnings for language\n
	    # features that should no longer be used and will be removed in future versions.\n
	    # To disable these warnings, set the following value to False:
	    #\n
	    #deprecation_warnings = True\n
	    \n
	    # Ansible can optionally warn when usage of the shell and\n
	    # command module appear to be simplified by using a default Ansible module\n
	    # instead. These warnings can be silenced by adjusting the following\n
	    # setting or adding warn=yes or warn=no to the end of the command line\n
	    # parameter string. This will for example suggest using the git module\n
	    # instead of shelling out to the git command.\n
	    #\n
	    #command_warnings = False\n
	    \n
	    \n
	    # set plugin path directories here, separate with colons\n
	    #action_plugins     = /usr/share/ansible/plugins/action\n
	    #become_plugins     = /usr/share/ansible/plugins/become\n
	    #cache_plugins      = /usr/share/ansible/plugins/cache\n
	    #callback_plugins   = /usr/share/ansible/plugins/callback\n
	    #connection_plugins = /usr/share/ansible/plugins/connection\n
	    #lookup_plugins     = /usr/share/ansible/plugins/lookup\n
	    #inventory_plugins  = /usr/share/ansible/plugins/inventory\n
	    #vars_plugins       = /usr/share/ansible/plugins/vars\n
	    #filter_plugins     = /usr/share/ansible/plugins/filter\n
	    #test_plugins       = /usr/share/ansible/plugins/test\n
	    #terminal_plugins   = /usr/share/ansible/plugins/terminal\n
	    #strategy_plugins   = /usr/share/ansible/plugins/strategy\n
	    \n
	    \n
	    # Ansible will use the 'linear' strategy but you may want to try another one.\n
	    #strategy = linear\n
	    \n
	    # By default, callbacks are not loaded for /bin/ansible. Enable this if you\n
	    # want, for example, a notification or logging callback to also apply to\n
	    # /bin/ansible runs\n
	    #\n
	    #bin_ansible_callbacks = False\n
	    \n
	    \n
	    # Don't like cows?  that's unfortunate.\n
	    # set to 1 if you don't want cowsay support or export ANSIBLE_NOCOWS=1\n
	    #nocows = 1\n
	    \n
	    # Set which cowsay stencil you'd like to use by default. When set to 'random',\n
	    # a random stencil will be selected for each task. The selection will be filtered\n
	    # against the `cow_enabled` option below.\n
	    #\n
	    #cow_selection = default\n
	    #cow_selection = random\n
	    \n
	    # When using the 'random' option for cowsay, stencils will be restricted to this list.\n
	    # it should be formatted as a comma-separated list with no spaces between names.\n
	    # NOTE: line continuations here are for formatting purposes only, as the INI parser\n
	    #       in python does not support them.\n
	    #\n
	    #cowsay_enabled_stencils=bud-frogs,bunny,cheese,daemon,default,dragon,elephant-in-snake,elephant,eyes,\\n
	    #              hellokitty,kitty,luke-koala,meow,milk,moofasa,moose,ren,sheep,small,stegosaurus,\\n
	    #              stimpy,supermilker,three-eyes,turkey,turtle,tux,udder,vader-koala,vader,www\n
	    \n
	    # Don't like colors either?\n
	    # set to 1 if you don't want colors, or export ANSIBLE_NOCOLOR=1\n
	    #\n
	    #nocolor = 1\n
	    \n
	    # If set to a persistent type (not 'memory', for example 'redis') fact values\n
	    # from previous runs in Ansible will be stored. This may be useful when\n
	    # wanting to use, for example, IP information from one group of servers\n
	    # without having to talk to them in the same playbook run to get their\n
	    # current IP information.\n
	    #\n
	    #fact_caching = memory\n
	    \n
	    # This option tells Ansible where to cache facts. The value is plugin dependent.\n
	    # For the jsonfile plugin, it should be a path to a local directory.\n
	    # For the redis plugin, the value is a host:port:database triplet: fact_caching_connection = localhost:6379:0
	    #\n
	    #fact_caching_connection=/tmp\n
	    \n
	    # retry files\n
	    # When a playbook fails a .retry file can be created that will be placed in ~/\n
	    # You can enable this feature by setting retry_files_enabled to True\n
	    # and you can change the location of the files by setting retry_files_save_path\n
	    #\n
	    #retry_files_enabled = False\n
	    #retry_files_save_path = ~/.ansible-retry\n
	    \n
	    # prevents logging of task data, off by default\n
	    #no_log = False\n
	    \n
	    # prevents logging of tasks, but only on the targets, data is still logged on the master/controller\n
	    #no_target_syslog = False\n
	    \n
	    # Controls whether Ansible will raise an error or warning if a task has no\n
	    # choice but to create world readable temporary files to execute a module on\n
	    # the remote machine. This option is False by default for security. Users may\n
	    # turn this on to have behaviour more like Ansible prior to 2.1.x. See\n
	    # https://docs.ansible.com/ansible/latest/user_guide/become.html#risks-of-becoming-an-unprivileged-user\n
	    # for more secure ways to fix this than enabling this option.\n
	    #\n
	    #allow_world_readable_tmpfiles = False\n
	    \n
	    # Controls what compression method is used for new-style ansible modules when\n
	    # they are sent to the remote system. The compression types depend on having\n
	    # support compiled into both the controller's python and the client's python.\n
	    # The names should match with the python Zipfile compression types:
	    # * ZIP_STORED (no compression. available everywhere)\n
	    # * ZIP_DEFLATED (uses zlib, the default)\n
	    # These values may be set per host via the ansible_module_compression inventory variable.\n
	    #\n
	    #module_compression = 'ZIP_DEFLATED'\n
	    \n
	    # This controls the cutoff point (in bytes) on --diff for files\n
	    # set to 0 for unlimited (RAM may suffer!).\n
	    #\n
	    #max_diff_size = 104448\n
	    \n
	    # Controls showing custom stats at the end, off by default\n
	    #show_custom_stats = False\n
	    \n
	    # Controls which files to ignore when using a directory as inventory with\n
	    # possibly multiple sources (both static and dynamic)\n
	    #\n
	    #inventory_ignore_extensions = ~, .orig, .bak, .ini, .cfg, .retry, .pyc, .pyo\n
	    \n
	    # This family of modules use an alternative execution path optimized for network appliances\n
	    # only update this setting if you know how this works, otherwise it can break module execution\n
	    #\n
	    #network_group_modules=eos, nxos, ios, iosxr, junos, vyos\n
	    \n
	    # When enabled, this option allows lookups (via variables like {{lookup('foo')}} or when used as\n
	    # a loop with `with_foo`) to return data that is not marked "unsafe". This means the data may contain\n
	    # jinja2 templating language which will be run through the templating engine.\n
	    # ENABLING THIS COULD BE A SECURITY RISK\n
	    #\n
	    #allow_unsafe_lookups = False\n
	    \n
	    # set default errors for all plays\n
	    #any_errors_fatal = False\n
	    \n
	    \n
	    [inventory]\n
	    # List of enabled inventory plugins and the order in which they are used.\n
	    enable_plugins = host_list, script, auto, yaml, ini, toml\n
	    \n
	    # Ignore these extensions when parsing a directory as inventory source\n
	    #ignore_extensions = .pyc, .pyo, .swp, .bak, ~, .rpm, .md, .txt, ~, .orig, .ini, .cfg, .retry\n
	    \n
	    # ignore files matching these patterns when parsing a directory as inventory source\n
	    #ignore_patterns=\n
	    \n
	    # If 'True' unparsed inventory sources become fatal errors, otherwise they are warnings.\n
	    #unparsed_is_failed = False\n
	    \n
	    \n
	    [privilege_escalation]\n
	    become = True\n
	    become_user = root\n
	    become_ask_pass = False\n
	    \n
	    \n
	    ## Connection Plugins ##\n
	    \n
	    # Settings for each connection plugin go under a section titled '[[plugin_name]_connection]'\n
	    # To view available connection plugins, run ansible-doc -t connection -l\n
	    # To view available options for a connection plugin, run ansible-doc -t connection [plugin_name]\n
	    # https://docs.ansible.com/ansible/latest/plugins/connection.html\n
	    \n
	    [paramiko_connection]\n
	    # uncomment this line to cause the paramiko connection plugin to not record new host\n
	    # keys encountered. Increases performance on new host additions. Setting works independently of the\n
	    # host key checking setting above.\n
	    #record_host_keys=False\n
	    \n
	    # by default, Ansible requests a pseudo-terminal for commands executed under sudo. Uncomment this\n
	    # line to disable this behaviour.\n
	    #pty = False\n
	    \n
	    # paramiko will default to looking for SSH keys initially when trying to\n
	    # authenticate to remote devices. This is a problem for some network devices\n
	    # that close the connection after a key failure. Uncomment this line to\n
	    # disable the Paramiko look for keys function\n
	    #look_for_keys = False\n
	    \n
	    # When using persistent connections with Paramiko, the connection runs in a\n
	    # background process. If the host doesn't already have a valid SSH key, by\n
	    # default Ansible will prompt to add the host key. This will cause connections\n
	    # running in background processes to fail. Uncomment this line to have\n
	    # Paramiko automatically add host keys.\n
	    #host_key_auto_add = True\n
	    \n
	    \n
	    [ssh_connection]\n
	    # ssh arguments to use\n
	    # Leaving off ControlPersist will result in poor performance, so use\n
	    # paramiko on older platforms rather than removing it, -C controls compression use\n
	    #ssh_args = -C -o ControlMaster=auto -o ControlPersist=60s\n
	    \n
	    # The base directory for the ControlPath sockets.\n
	    # This is the "%(directory)s" in the control_path option\n
	    #\n
	    # Example:
	    # control_path_dir = /tmp/.ansible/cp\n
	    #control_path_dir = ~/.ansible/cp\n
	    \n
	    # The path to use for the ControlPath sockets. This defaults to a hashed string of the hostname,\n
	    # port and username (empty string in the config). The hash mitigates a common problem users\n
	    # found with long hostnames and the conventional %(directory)s/ansible-ssh-%%h-%%p-%%r format.\n
	    # In those cases, a "too long for Unix domain socket" ssh error would occur.\n
	    #\n
	    # Example:
	    # control_path = %(directory)s/%%C\n
	    #control_path =\n
	    \n
	    # Enabling pipelining reduces the number of SSH operations required to\n
	    # execute a module on the remote server. This can result in a significant\n
	    # performance improvement when enabled, however when using "sudo:" you must\n
	    # first disable 'requiretty' in /etc/sudoers\n
	    #\n
	    # By default, this option is disabled to preserve compatibility with\n
	    # sudoers configurations that have requiretty (the default on many distros).\n
	    #\n
	    #pipelining = False\n
	    \n
	    # Control the mechanism for transferring files (old)\n
	    #   * smart = try sftp and then try scp [default]\n
	    #   * True = use scp only\n
	    #   * False = use sftp only\n
	    #scp_if_ssh = smart\n
	    \n
	    # Control the mechanism for transferring files (new)\n
	    # If set, this will override the scp_if_ssh option\n
	    #   * sftp  = use sftp to transfer files\n
	    #   * scp   = use scp to transfer files\n
	    #   * piped = use 'dd' over SSH to transfer files\n
	    #   * smart = try sftp, scp, and piped, in that order [default]\n
	    #transfer_method = smart\n
	    \n
	    # If False, sftp will not use batch mode to transfer files. This may cause some\n
	    # types of file transfer failures impossible to catch however, and should\n
	    # only be disabled if your sftp version has problems with batch mode\n
	    #sftp_batch_mode = False\n
	    \n
	    # The -tt argument is passed to ssh when pipelining is not enabled because sudo\n
	    # requires a tty by default.\n
	    #usetty = True\n
	    \n
	    # Number of times to retry an SSH connection to a host, in case of UNREACHABLE.\n
	    # For each retry attempt, there is an exponential backoff,\n
	    # so after the first attempt there is 1s wait, then 2s, 4s etc. up to 30s (max).\n
	    #retries = 3\n
	    \n
	    \n
	    [persistent_connection]\n
	    # Configures the persistent connection timeout value in seconds. This value is\n
	    # how long the persistent connection will remain idle before it is destroyed.\n
	    # If the connection doesn't receive a request before the timeout value\n
	    # expires, the connection is shutdown. The default value is 30 seconds.\n
	    #connect_timeout = 30\n
	    \n
	    # The command timeout value defines the amount of time to wait for a command\n
	    # or RPC call before timing out. The value for the command timeout must\n
	    # be less than the value of the persistent connection idle timeout (connect_timeout)\n
	    # The default value is 30 second.\n
	    #command_timeout = 30\n
	    \n
	    \n
	    ## Become Plugins ##\n
	    \n
	    # Settings for become plugins go under a section named '[[plugin_name]_become_plugin]'\n
	    # To view available become plugins, run ansible-doc -t become -l\n
	    # To view available options for a specific plugin, run ansible-doc -t become [plugin_name]\n
	    # https://docs.ansible.com/ansible/latest/plugins/become.html\n
	    \n
	    [sudo_become_plugin]\n
	    #flags = -H -S -n\n
	    #user = root\n
	    \n
	    \n
	    [selinux]\n
	    # file systems that require special treatment when dealing with security context\n
	    # the default behaviour that copies the existing context or uses the user default\n
	    # needs to be changed to use the file system dependent context.\n
	    #special_context_filesystems=fuse,nfs,vboxsf,ramfs,9p,vfat\n
	    \n
	    # Set this to True to allow libvirt_lxc connections to work without SELinux.\n
	    #libvirt_lxc_noseclabel = False\n
	    \n
	    \n
	    [colors]\n
	    highlight = white\n
	    verbose = blue\n
	    warn = bright purple\n
	    error = red\n
	    debug = dark gray\n
	    deprecate = purple\n
	    skip = cyan\n
	    unreachable = red\n
	    ok = green\n
	    changed = yellow\n
	    diff_add = green\n
	    diff_remove = red\n
	    diff_lines = cyan\n
	    \n
	    \n
	    [diff]\n
	    # Always print diff when running ( same as always running with -D/--diff )\n
	    #always = False\n
	    \n
	    # Set how many context lines to show in diff\n
	    #context = 3\n
	    \n
	    [galaxy]\n
	    # Controls whether the display wheel is shown or not\n
	    #display_progress=\n
	    \n
	    # Validate TLS certificates for Galaxy server\n
	    #ignore_certs = False\n
	    \n
	    # Role or collection skeleton directory to use as a template for\n
	    # the init action in ansible-galaxy command\n
	    #role_skeleton=\n
	    \n
	    # Patterns of files to ignore inside a Galaxy role or collection\n
	    # skeleton directory\n
	    #role_skeleton_ignore="^.git$", "^.*/.git_keep$"\n
	    \n
	    # Galaxy Server URL\n
	    #server=https://galaxy.ansible.com\n
	    \n
	    # A list of Galaxy servers to use when installing a collection.\n
	    #server_list=automation_hub, release_galaxy\n
	    \n
	    # Server specific details which are mentioned in server_list\n
	    #[galaxy_server.automation_hub]\n
	    #url=https://cloud.redhat.com/api/automation-hub/\n
	    #auth_url=https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token\n
	    #token=my_ah_token\n
	    #\n
	    #[galaxy_server.release_galaxy]\n
	    #url=https://galaxy.ansible.com/\n
	    #token=my_token\n
	"""#
