function greenHtml(text) {
  return '<div style="color:green;">' + text + '</div>';
}

function redHtml(text) {
  return '<div style="color:red;">' + text + '</div>';
}

$(function() {
  $("#ruleTestExecute").click(function() {
    var text = ace.edit("ruleTextArea").getValue();
    $("#ruleTestInfo").html("Testing...");          

    $.ajax({
      method: 'POST',
      url: PATH_PREFIX + "/api/v1/alerts_testing",
      dataType: "json",
      data: JSON.stringify({
        RuleText: encodeURIComponent(text)
      }),
      success: function(data) {
        data = data.data
        if(data.IsError) {
          var errStr = "Syntax of the file is erroneous<br/>Error message:<br/>"
          var len = data.Errors.length 
          for(var i = 0; i<len; i++) {
            errStr += "(" + (i+1) + ") " + data.Errors[i] + '<br/>'
          }
          $("#ruleTestInfo").html(redHtml(errStr));
        } else {
          $("#ruleTestInfo").html(greenHtml(data.Success));          
        }
      },
      error: function(jqXHR, textStatus, errorThrown) {
        $("#ruleTestInfo").html(redHtml("ERROR: "+errorThrown));
      }
    });
    
  });
  
});
  