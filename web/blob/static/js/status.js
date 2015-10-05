/* globals PATH_PREFIX, jQuery */
(function ($, undefined) {
    'use strict';

    $('#reload-conf-btn').on('click', function () {
        var elBtn;
        $(this).promise().then(function (btn) {
            elBtn = btn;
            elBtn.text('Waiting...').prop('disabled', true);
            return $.post(PATH_PREFIX + '/-/reload');
        }).then(function () {
            elBtn.text('Loaded Successfully!')
                .prop('disabled', false)
                .addClass('btn-success');
        }, function() {
            elBtn.text('Failed to load!')
                .prop('disabled', false)
                .addClass('btn-danger');
        }).always(function () {
            setTimeout(function () {
                elBtn.removeClass('btn-danger btn-success')
                    .text('Reload Configurations');
            }, 1000);
        });
    });
}(jQuery));