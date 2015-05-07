$(function() {
  var addBtn    = $("#add-btn")
  var modal     = $(".modal")
  var tableBody = $("table tbody")
  var alertBox  = $(".alert")
  var template  = _.template($("#row-template").html())

  $(document).ajaxError(function(event, jqxhr, settings, error) {
    var msg = settings.type + " " + settings.url + " Error!"

    alertBox.removeClass("hide").find("p").text(msg)
  })

  $.getJSON("/repos.json", function(repos) {
    _.each(repos, function(repo) {
      repo.created_at = moment(repo.created_at)
      tableBody.append(template(repo))
    })
  })

  addBtn.click(function(e) {
    e.preventDefault()
    modal.removeClass("hide").modal()
  })
})
