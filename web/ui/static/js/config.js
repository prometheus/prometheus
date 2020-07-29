function init() {
    $("#copyToClipboard").on("click", function () {
        var range = document.createRange();
        range.selectNode(document.getElementById("config_yaml"));
        window.getSelection().empty();
        window.getSelection().addRange(range);
        document.execCommand("copy");
        window.getSelection().empty();
    });
}

$(init);
