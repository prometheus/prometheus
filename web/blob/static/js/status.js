/* globals PATH_PREFIX, $ */
$(function () {
    'use strict';

    $('#reload_conf_btn').on('click', function () {
        var elBtn;
        $(this).promise().then(function (btn) {
            elBtn = btn;
            elBtn.text('Waiting...').prop('disabled', true);
            return $.post(PATH_PREFIX + '/-/reload');
        }).then(function () {
            elBtn.text('Reloaded Successfully!')
                .prop('disabled', false)
                .addClass('btn-success');
        }, function() {
            // todo: show verbose error (the body itself)
            elBtn.text('Failed to reload!')
                .prop('disabled', false)
                .addClass('btn-danger');
        }).always(function () {
            setTimeout(function () {
                elBtn.removeClass('btn-danger btn-success')
                    .text('Reload Configuration');
            }, 1400);
        });
    });
});
