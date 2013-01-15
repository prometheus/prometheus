function submitQuery() {
  var form = $("#queryForm");

  $.ajax({
        method: form.attr("method"),
        url: form.attr("action"),
        dataType: "html",
        data: form.serialize(),
        success: function(data, textStatus) {
          $("#result").text(data);
        },
        error: function() {
          alert("Error executing query!");
        },
  });
  return false;
}

function bindHandlers() {
  jQuery.ajaxSetup({
      cache: false
  });
  $("#queryForm").submit(submitQuery);
}

$(bindHandlers);
