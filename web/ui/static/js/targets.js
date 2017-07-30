function toggle(obj, state){
    var icon = $(obj).find("i");
    if (icon.length === 0 ) {
        return;
    }

    if (state === true) {
        icon.removeClass("icon-chevron-down").addClass("icon-chevron-up");
    } else {
        icon.removeClass("icon-chevron-up").addClass("icon-chevron-down");
    }

    $(obj).next().toggle(state);
}

function init() {
    $(".job_header").click(function() {
        var job = $(this).find("a").attr("id"),
            expanderIcon = $(this).find("i.icon-chevron-down");

        if (expanderIcon.length !== 0) {
            localStorage.setItem(job, false);
            toggle(this, true);
        } else {
            localStorage.setItem(job, true);
            toggle(this, false);
        }
    });

    $(".job_header a").each(function(i, obj) {
        var selector = $(obj).attr("id");
        if (localStorage.getItem(selector) === "true") {
          toggle($(this).parents(".job_header"), false);
        }
    });
}

$(init);