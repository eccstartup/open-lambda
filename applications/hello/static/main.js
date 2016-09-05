var config;

function lambda_post(data, callback) {
  var url = config['url']

  $.ajax({
    type: "POST",
    url: url,
    contentType: "application/json; charset=utf-8",
    data: JSON.stringify(data),
    dataType: "json",
    success: callback,
    failure: function(error) {
      $("#comments").html("Error: " + error + ".  Consider refreshing.")
    }
  });  
}

function echo() {
  var msg = $("#msg").val();
  $("#msg").val("");
  lambda_post({"op":"hello", "msg":msg}, function(data){
    alert(data.result);
  });
}

function main() {
  $("#submit").click(echo);
  $.getJSON('config.json')
    .done(function(data) {
      config = data;
    })
    .fail(function( jqxhr, textStatus, error ) {
      $("#comments").html("Error: " + error + ".  Consider refreshing.")
    })
}

$(document).ready(function() {
  main();
});